package cli

import (
	"fmt"

	"github.com/go-playground/validator/v10"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
)

func userCreate() *clibase.Command {
	var (
		email    string
		username string
		password string
	)
	cmd := &clibase.Command{
		Use: "create",
		Handler: func(inv *clibase.Invokation) error {
			client, err := useClient(cmd)
			if err != nil {
				return err
			}
			organization, err := CurrentOrganization(cmd, client)
			if err != nil {
				return err
			}
			if username == "" {
				username, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text: "Username:",
				})
				if err != nil {
					return err
				}
			}
			if email == "" {
				email, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text: "Email:",
					Validate: func(s string) error {
						err := validator.New().Var(s, "email")
						if err != nil {
							return xerrors.New("That's not a valid email address!")
						}
						return err
					},
				})
				if err != nil {
					return err
				}
			}
			if password == "" {
				password, err = cryptorand.StringCharset(cryptorand.Human, 20)
				if err != nil {
					return err
				}
			}

			_, err = client.CreateUser(inv.Context(), codersdk.CreateUserRequest{
				Email:          email,
				Username:       username,
				Password:       password,
				OrganizationID: organization.ID,
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(inv.Stderr, `A new user has been created!
Share the instructions below to get them started.
`+cliui.Styles.Placeholder.Render("—————————————————————————————————————————————————")+`
Download the Coder command line for your operating system:
https://github.com/coder/coder/releases

Run `+cliui.Styles.Code.Render("coder login "+client.URL.String())+` to authenticate.

Your email is: `+cliui.Styles.Field.Render(email)+`
Your password is: `+cliui.Styles.Field.Render(password)+`

Create a workspace  `+cliui.Styles.Code.Render("coder create")+`!`)
			return nil
		},
	}
	cmd.Flags().StringVarP(&email, "email", "e", "", "Specifies an email address for the new user.")
	cmd.Flags().StringVarP(&username, "username", "u", "", "Specifies a username for the new user.")
	cmd.Flags().StringVarP(&password, "password", "p", "", "Specifies a password for the new user.")
	return cmd
}
