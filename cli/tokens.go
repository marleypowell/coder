package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func tokens() *clibase.Command {
	cmd := &clibase.Command{
		Use:     "tokens",
		Short:   "Manage personal access tokens",
		Long:    "Tokens are used to authenticate automated clients to Coder.",
		Aliases: []string{"token"},
		Example: formatExamples(
			example{
				Description: "Create a token for automation",
				Command:     "coder tokens create",
			},
			example{
				Description: "List your tokens",
				Command:     "coder tokens ls",
			},
			example{
				Description: "Remove a token by ID",
				Command:     "coder tokens rm WuoWs4ZsMX",
			},
		),
		Handler: func(inv *clibase.Invokation) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		createToken(),
		listTokens(),
		removeToken(),
	)

	return cmd
}

func createToken() *clibase.Command {
	var (
		tokenLifetime time.Duration
		name          string
	)
	cmd := &clibase.Command{
		Use:   "create",
		Short: "Create a token",
		Handler: func(inv *clibase.Invokation) error {
			client, err := useClient(cmd)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			res, err := client.CreateToken(inv.Context(), codersdk.Me, codersdk.CreateTokenRequest{
				Lifetime:  tokenLifetime,
				TokenName: name,
			})
			if err != nil {
				return xerrors.Errorf("create tokens: %w", err)
			}

			cmd.Println(cliui.Styles.Wrap.Render(
				"Here is your token. 🪄",
			))
			cmd.Println()
			cmd.Println(cliui.Styles.Code.Render(strings.TrimSpace(res.Key)))
			cmd.Println()
			cmd.Println(cliui.Styles.Wrap.Render(
				fmt.Sprintf("You can use this token by setting the --%s CLI flag, the %s environment variable, or the %q HTTP header.", varToken, envSessionToken, codersdk.SessionTokenHeader),
			))

			return nil
		},
	}

	cliflag.DurationVarP(cmd.Flags(), &tokenLifetime, "lifetime", "", "CODER_TOKEN_LIFETIME", 30*24*time.Hour, "Specify a duration for the lifetime of the token.")
	cmd.Flags().StringVarP(&name, "name", "n", "", "Specify a human-readable name.")

	return cmd
}

// tokenListRow is the type provided to the OutputFormatter.
type tokenListRow struct {
	// For JSON format:
	codersdk.APIKey `table:"-"`

	// For table format:
	ID        string    `json:"-" table:"id,default_sort"`
	TokenName string    `json:"token_name" table:"name"`
	LastUsed  time.Time `json:"-" table:"last used"`
	ExpiresAt time.Time `json:"-" table:"expires at"`
	CreatedAt time.Time `json:"-" table:"created at"`
	Owner     string    `json:"-" table:"owner"`
}

func tokenListRowFromToken(token codersdk.APIKeyWithOwner) tokenListRow {
	return tokenListRow{
		APIKey:    token.APIKey,
		ID:        token.ID,
		TokenName: token.TokenName,
		LastUsed:  token.LastUsed,
		ExpiresAt: token.ExpiresAt,
		CreatedAt: token.CreatedAt,
		Owner:     token.Username,
	}
}

func listTokens() *clibase.Command {
	// we only display the 'owner' column if the --all argument is passed in
	defaultCols := []string{"id", "name", "last used", "expires at", "created at"}
	if slices.Contains(os.Args, "-a") || slices.Contains(os.Args, "--all") {
		defaultCols = append(defaultCols, "owner")
	}

	var (
		all           bool
		displayTokens []tokenListRow
		formatter     = cliui.NewOutputFormatter(
			cliui.TableFormat([]tokenListRow{}, defaultCols),
			cliui.JSONFormat(),
		)
	)
	cmd := &clibase.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List tokens",
		Handler: func(inv *clibase.Invokation) error {
			client, err := useClient(cmd)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			tokens, err := client.Tokens(inv.Context(), codersdk.Me, codersdk.TokensFilter{
				IncludeAll: all,
			})
			if err != nil {
				return xerrors.Errorf("list tokens: %w", err)
			}

			if len(tokens) == 0 {
				cmd.Println(cliui.Styles.Wrap.Render(
					"No tokens found.",
				))
			}

			displayTokens = make([]tokenListRow, len(tokens))

			for i, token := range tokens {
				displayTokens[i] = tokenListRowFromToken(token)
			}

			out, err := formatter.Format(inv.Context(), displayTokens)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	cmd.Flags().BoolVarP(&all, "all", "a", false,
		"Specifies whether all users' tokens will be listed or not (must have Owner role to see all tokens).")

	formatter.AttachFlags(cmd)
	return cmd
}

func removeToken() *clibase.Command {
	cmd := &clibase.Command{
		Use:        "remove [name]",
		Aliases:    []string{"rm"},
		Short:      "Delete a token",
		Middleware: clibase.RequireNArgs(1),,
		Handler: func(inv *clibase.Invokation) error {
			client, err := useClient(cmd)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			token, err := client.APIKeyByName(inv.Context(), codersdk.Me, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("fetch api key by name %s: %w", inv.Args[0], err)
			}

			err = client.DeleteAPIKey(inv.Context(), codersdk.Me, token.ID)
			if err != nil {
				return xerrors.Errorf("delete api key: %w", err)
			}

			cmd.Println(cliui.Styles.Wrap.Render(
				"Token has been deleted.",
			))

			return nil
		},
	}

	return cmd
}
