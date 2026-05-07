package commands

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/spf13/cobra"
)

const schemaCompactJSON = `{
  "version": "1",
  "commands": [
    {"name": "login", "args": []},
    {"name": "logout", "args": []},
    {"name": "whoami", "args": []},
    {"name": "templates", "args": []},
    {"name": "templates show", "args": ["name|id"]},
    {"name": "create", "required_flags": ["template","autofill"]},
    {"name": "list", "flags": ["fields","limit"]},
    {"name": "export", "args": ["name|id"], "required_flags": ["format"]},
    {"name": "folders", "args": []},
    {"name": "schema", "flags": ["compact","full","command"]},
    {"name": "sql", "args": ["query"], "flags": ["limit"]}
  ],
  "exit_codes": {"0":"success","2":"auth","3":"not_found","4":"network","5":"validation","6":"rate_limited","7":"permission_denied"}
}`

const schemaFullJSON = `{
  "version": "1",
  "commands": [
    {"name":"login","summary":"OAuth 2.0 PKCE browser flow","examples":["canva login"]},
    {"name":"logout","summary":"Remove stored credentials and clear cache"},
    {"name":"whoami","summary":"Show authenticated user","examples":["canva whoami"]},
    {"name":"templates","summary":"List brand templates (Enterprise)","examples":["canva templates"]},
    {"name":"templates show","summary":"Get autofill dataset for template","args":["name|id"],"examples":["canva templates show 'Social Post'"]},
    {"name":"create","summary":"Create design from template + autofill (Enterprise)","required_flags":["template","autofill"],"flags":["folder","title","idempotency-key","dry-run"],"examples":["canva create --template 'Social Post' --autofill data.json"],"error_codes":["template_not_found","validation","permission_denied"]},
    {"name":"list","summary":"List designs as NDJSON","flags":["fields","limit"],"examples":["canva list --limit 5","canva list --fields id,title"]},
    {"name":"export","summary":"Export design (eager download)","args":["name|id"],"required_flags":["format"],"flags":["output","url-only"],"examples":["canva export 'Q3 Banner' --format pdf"],"error_codes":["design_not_found","validation"]},
    {"name":"folders","summary":"List folders by walking root and uploads"},
    {"name":"schema","summary":"Print this schema","flags":["compact","full","command"]},
    {"name":"sql","summary":"Read-only SQL against local cache","args":["query"],"flags":["limit"],"examples":["canva sql \"SELECT id,title FROM designs LIMIT 5\""]}
  ],
  "global_flags": ["json","no-cache","quiet","auto-wait"],
  "exit_codes": {"0":"success","2":"auth_required/auth_revoked","3":"not_found","4":"network","5":"validation","6":"rate_limited","7":"permission_denied"},
  "error_envelope": {"error":"<stable code>","message":"<human>","fix":"<literal command to retry>","exit_code":1}
}`

func NewSchema() *cobra.Command {
	var compact, full bool
	var command string
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Print the canvacli schema as JSON for agent introspection",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := schemaCompactJSON
			if full {
				out = schemaFullJSON
			} else if command != "" {
				return errors.New("schema --command not yet implemented; use --full and filter externally")
			}
			var v any
			if err := json.Unmarshal([]byte(out), &v); err != nil {
				return err
			}
			b, _ := json.Marshal(v)
			os.Stdout.Write(append(b, '\n'))
			return nil
		},
	}
	cmd.Flags().BoolVar(&compact, "compact", true, "compact schema (~500 tokens)")
	cmd.Flags().BoolVar(&full, "full", false, "full schema (~3K tokens)")
	cmd.Flags().StringVar(&command, "command", "", "schema for one command only")
	return cmd
}
