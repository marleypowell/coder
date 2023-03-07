package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
)

func create() *clibase.Command {
	var (
		parameterFile     string
		richParameterFile string
		templateName      string
		startAt           string
		stopAfter         time.Duration
		workspaceName     string
	)
	cmd := &clibase.Command{
		Annotations: workspaceCommand,
		Use:         "create [name]",
		Short:       "Create a workspace",
		Handler: func(inv *clibase.Invokation) error {
			client, err := useClient(cmd)
			if err != nil {
				return err
			}

			organization, err := CurrentOrganization(cmd, client)
			if err != nil {
				return err
			}

			if len(inv.Args) >= 1 {
				workspaceName = inv.Args[0]
			}

			if workspaceName == "" {
				workspaceName, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text: "Specify a name for your workspace:",
					Validate: func(workspaceName string) error {
						_, err = client.WorkspaceByOwnerAndName(inv.Context(), codersdk.Me, workspaceName, codersdk.WorkspaceOptions{})
						if err == nil {
							return xerrors.Errorf("A workspace already exists named %q!", workspaceName)
						}
						return nil
					},
				})
				if err != nil {
					return err
				}
			}

			_, err = client.WorkspaceByOwnerAndName(inv.Context(), codersdk.Me, workspaceName, codersdk.WorkspaceOptions{})
			if err == nil {
				return xerrors.Errorf("A workspace already exists named %q!", workspaceName)
			}

			var template codersdk.Template
			if templateName == "" {
				_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Wrap.Render("Select a template below to preview the provisioned infrastructure:"))

				templates, err := client.TemplatesByOrganization(inv.Context(), organization.ID)
				if err != nil {
					return err
				}

				slices.SortFunc(templates, func(a, b codersdk.Template) bool {
					return a.ActiveUserCount > b.ActiveUserCount
				})

				templateNames := make([]string, 0, len(templates))
				templateByName := make(map[string]codersdk.Template, len(templates))

				for _, template := range templates {
					templateName := template.Name

					if template.ActiveUserCount > 0 {
						templateName += cliui.Styles.Placeholder.Render(
							fmt.Sprintf(
								" (used by %s)",
								formatActiveDevelopers(template.ActiveUserCount),
							),
						)
					}

					templateNames = append(templateNames, templateName)
					templateByName[templateName] = template
				}

				// Move the cursor up a single line for nicer display!
				option, err := cliui.Select(inv, cliui.SelectOptions{
					Options:    templateNames,
					HideSearch: true,
				})
				if err != nil {
					return err
				}

				template = templateByName[option]
			} else {
				template, err = client.TemplateByName(inv.Context(), organization.ID, templateName)
				if err != nil {
					return xerrors.Errorf("get template by name: %w", err)
				}
			}

			var schedSpec *string
			if startAt != "" {
				sched, err := parseCLISchedule(startAt)
				if err != nil {
					return err
				}
				schedSpec = ptr.Ref(sched.String())
			}

			buildParams, err := prepWorkspaceBuild(cmd, client, prepWorkspaceBuildArgs{
				Template:          template,
				ExistingParams:    []codersdk.Parameter{},
				ParameterFile:     parameterFile,
				RichParameterFile: richParameterFile,
				NewWorkspaceName:  workspaceName,
			})
			if err != nil {
				return err
			}

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Confirm create?",
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			workspace, err := client.CreateWorkspace(inv.Context(), organization.ID, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID:          template.ID,
				Name:                workspaceName,
				AutostartSchedule:   schedSpec,
				TTLMillis:           ptr.Ref(stopAfter.Milliseconds()),
				ParameterValues:     buildParams.parameters,
				RichParameterValues: buildParams.richParameters,
			})
			if err != nil {
				return err
			}

			err = cliui.WorkspaceBuild(inv.Context(), inv.Stdout, client, workspace.LatestBuild.ID)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(inv.Stdout, "\nThe %s workspace has been created at %s!\n", cliui.Styles.Keyword.Render(workspace.Name), cliui.Styles.DateTimeStamp.Render(time.Now().Format(time.Stamp)))
			return nil
		},
	}

	cliui.AllowSkipPrompt(inv)
	cliflag.StringVarP(cmd.Flags(), &templateName, "template", "t", "CODER_TEMPLATE_NAME", "", "Specify a template name.")
	cliflag.StringVarP(cmd.Flags(), &parameterFile, "parameter-file", "", "CODER_PARAMETER_FILE", "", "Specify a file path with parameter values.")
	cliflag.StringVarP(cmd.Flags(), &richParameterFile, "rich-parameter-file", "", "CODER_RICH_PARAMETER_FILE", "", "Specify a file path with values for rich parameters defined in the template.")
	cliflag.StringVarP(cmd.Flags(), &startAt, "start-at", "", "CODER_WORKSPACE_START_AT", "", "Specify the workspace autostart schedule. Check `coder schedule start --help` for the syntax.")
	cliflag.DurationVarP(cmd.Flags(), &stopAfter, "stop-after", "", "CODER_WORKSPACE_STOP_AFTER", 8*time.Hour, "Specify a duration after which the workspace should shut down (e.g. 8h).")
	return cmd
}

type prepWorkspaceBuildArgs struct {
	Template           codersdk.Template
	ExistingParams     []codersdk.Parameter
	ParameterFile      string
	ExistingRichParams []codersdk.WorkspaceBuildParameter
	RichParameterFile  string
	NewWorkspaceName   string

	UpdateWorkspace bool
}

type buildParameters struct {
	// Parameters contains legacy parameters stored in /parameters.
	parameters []codersdk.CreateParameterRequest
	// Rich parameters stores values for build parameters annotated with description, icon, type, etc.
	richParameters []codersdk.WorkspaceBuildParameter
}

// prepWorkspaceBuild will ensure a workspace build will succeed on the latest template version.
// Any missing params will be prompted to the user. It supports legacy and rich parameters.
func prepWorkspaceBuild(cmd *clibase.Command, client *codersdk.Client, inv.Args prepWorkspaceBuildArgs) (*buildParameters, error) {
	ctx := inv.Context()

	var useRichParameters bool
	if len(inv.Args.ExistingRichParams) > 0 && len(inv.Args.RichParameterFile) > 0 {
		useRichParameters = true
	}

	var useLegacyParameters bool
	if len(inv.Args.ExistingParams) > 0 || len(inv.Args.ParameterFile) > 0 {
		useLegacyParameters = true
	}

	if useRichParameters && useLegacyParameters {
		return nil, xerrors.Errorf("Rich parameters can't be used together with legacy parameters.")
	}

	templateVersion, err := client.TemplateVersion(ctx, inv.Args.Template.ActiveVersionID)
	if err != nil {
		return nil, err
	}

	// Legacy parameters
	parameterSchemas, err := client.TemplateVersionSchema(ctx, templateVersion.ID)
	if err != nil {
		return nil, err
	}

	// parameterMapFromFile can be nil if parameter file is not specified
	var parameterMapFromFile map[string]string
	useParamFile := false
	if inv.Args.ParameterFile != "" {
		useParamFile = true
		_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Paragraph.Render("Attempting to read the variables from the parameter file.")+"\r\n")
		parameterMapFromFile, err = createParameterMapFromFile(inv.Args.ParameterFile)
		if err != nil {
			return nil, err
		}
	}
	disclaimerPrinted := false
	legacyParameters := make([]codersdk.CreateParameterRequest, 0)
PromptParamLoop:
	for _, parameterSchema := range parameterSchemas {
		if !parameterSchema.AllowOverrideSource {
			continue
		}
		if !disclaimerPrinted {
			_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Paragraph.Render("This template has customizable parameters. Values can be changed after create, but may have unintended side effects (like data loss).")+"\r\n")
			disclaimerPrinted = true
		}

		// Param file is all or nothing
		if !useParamFile {
			for _, e := range inv.Args.ExistingParams {
				if e.Name == parameterSchema.Name {
					// If the param already exists, we do not need to prompt it again.
					// The workspace scope will reuse params for each build.
					continue PromptParamLoop
				}
			}
		}

		parameterValue, err := getParameterValueFromMapOrInput(cmd, parameterMapFromFile, parameterSchema)
		if err != nil {
			return nil, err
		}

		legacyParameters = append(legacyParameters, codersdk.CreateParameterRequest{
			Name:              parameterSchema.Name,
			SourceValue:       parameterValue,
			SourceScheme:      codersdk.ParameterSourceSchemeData,
			DestinationScheme: parameterSchema.DefaultDestinationScheme,
		})
	}

	if disclaimerPrinted {
		_, _ = fmt.Fprintln(inv.Stdout)
	}

	// Rich parameters
	templateVersionParameters, err := client.TemplateVersionRichParameters(inv.Context(), templateVersion.ID)
	if err != nil {
		return nil, xerrors.Errorf("get template version rich parameters: %w", err)
	}

	parameterMapFromFile = map[string]string{}
	useParamFile = false
	if inv.Args.RichParameterFile != "" {
		useParamFile = true
		_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Paragraph.Render("Attempting to read the variables from the rich parameter file.")+"\r\n")
		parameterMapFromFile, err = createParameterMapFromFile(inv.Args.RichParameterFile)
		if err != nil {
			return nil, err
		}
	}
	disclaimerPrinted = false
	richParameters := make([]codersdk.WorkspaceBuildParameter, 0)
PromptRichParamLoop:
	for _, templateVersionParameter := range templateVersionParameters {
		if !disclaimerPrinted {
			_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Paragraph.Render("This template has customizable parameters. Values can be changed after create, but may have unintended side effects (like data loss).")+"\r\n")
			disclaimerPrinted = true
		}

		// Param file is all or nothing
		if !useParamFile {
			for _, e := range inv.Args.ExistingRichParams {
				if e.Name == templateVersionParameter.Name {
					// If the param already exists, we do not need to prompt it again.
					// The workspace scope will reuse params for each build.
					continue PromptRichParamLoop
				}
			}
		}

		if inv.Args.UpdateWorkspace && !templateVersionParameter.Mutable {
			_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Warn.Render(fmt.Sprintf(`Parameter %q is not mutable, so can't be customized after workspace creation.`, templateVersionParameter.Name)))
			continue
		}

		parameterValue, err := getWorkspaceBuildParameterValueFromMapOrInput(cmd, parameterMapFromFile, templateVersionParameter)
		if err != nil {
			return nil, err
		}

		richParameters = append(richParameters, *parameterValue)
	}

	if disclaimerPrinted {
		_, _ = fmt.Fprintln(inv.Stdout)
	}

	err = cliui.GitAuth(ctx, inv.Stdout, cliui.GitAuthOptions{
		Fetch: func(ctx context.Context) ([]codersdk.TemplateVersionGitAuth, error) {
			return client.TemplateVersionGitAuth(ctx, templateVersion.ID)
		},
	})
	if err != nil {
		return nil, xerrors.Errorf("template version git auth: %w", err)
	}

	// Run a dry-run with the given parameters to check correctness
	dryRun, err := client.CreateTemplateVersionDryRun(inv.Context(), templateVersion.ID, codersdk.CreateTemplateVersionDryRunRequest{
		WorkspaceName:       inv.Args.NewWorkspaceName,
		ParameterValues:     legacyParameters,
		RichParameterValues: richParameters,
	})
	if err != nil {
		return nil, xerrors.Errorf("begin workspace dry-run: %w", err)
	}
	_, _ = fmt.Fprintln(inv.Stdout, "Planning workspace...")
	err = cliui.ProvisionerJob(inv.Context(), inv.Stdout, cliui.ProvisionerJobOptions{
		Fetch: func() (codersdk.ProvisionerJob, error) {
			return client.TemplateVersionDryRun(inv.Context(), templateVersion.ID, dryRun.ID)
		},
		Cancel: func() error {
			return client.CancelTemplateVersionDryRun(inv.Context(), templateVersion.ID, dryRun.ID)
		},
		Logs: func() (<-chan codersdk.ProvisionerJobLog, io.Closer, error) {
			return client.TemplateVersionDryRunLogsAfter(inv.Context(), templateVersion.ID, dryRun.ID, 0)
		},
		// Don't show log output for the dry-run unless there's an error.
		Silent: true,
	})
	if err != nil {
		// TODO (Dean): reprompt for parameter values if we deem it to
		// be a validation error
		return nil, xerrors.Errorf("dry-run workspace: %w", err)
	}

	resources, err := client.TemplateVersionDryRunResources(inv.Context(), templateVersion.ID, dryRun.ID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace dry-run resources: %w", err)
	}

	err = cliui.WorkspaceResources(inv.Stdout, resources, cliui.WorkspaceResourcesOptions{
		WorkspaceName: inv.Args.NewWorkspaceName,
		// Since agents haven't connected yet, hiding this makes more sense.
		HideAgentState: true,
		Title:          "Workspace Preview",
	})
	if err != nil {
		return nil, err
	}

	return &buildParameters{
		parameters:     legacyParameters,
		richParameters: richParameters,
	}, nil
}
