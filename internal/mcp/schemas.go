package mcp

// schemaCompact is a small JSON description of the canvacli surface, intended
// for agent introspection via the canva_schema MCP tool. Kept under 4 KB.
const schemaCompact = `{
  "version": "1",
  "mcp_tools": ["canva_whoami","canva_list","canva_folders","canva_export","canva_sql","canva_schema"],
  "cli_commands": [
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
    {"name": "sql", "args": ["query"], "flags": ["limit"]},
    {"name": "mcp serve", "args": []}
  ],
  "exit_codes": {"0":"success","2":"auth","3":"not_found","4":"network","5":"validation","6":"rate_limited","7":"permission_denied"}
}`

// schemaFull is a richer description including MCP tool argument shapes.
// Kept under 16 KB.
const schemaFull = `{
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
    {"name":"sql","summary":"Read-only SQL against local cache","args":["query"],"flags":["limit"],"examples":["canva sql \"SELECT id,title FROM designs LIMIT 5\""]},
    {"name":"mcp serve","summary":"Run an MCP server over stdio for Claude Desktop / Cursor / agents","examples":["canva mcp serve"]}
  ],
  "mcp_tools": [
    {"name":"canva_whoami","summary":"Authenticated user info","args":[]},
    {"name":"canva_list","summary":"List user's designs as JSON","args":[{"name":"limit","type":"number","default":20,"max":100},{"name":"fields","type":"string","default":"id,title,updated_at"}]},
    {"name":"canva_folders","summary":"Walk root + uploads folders","args":[]},
    {"name":"canva_export","summary":"Export a design and download to disk","args":[{"name":"design_id_or_name","type":"string","required":true},{"name":"format","type":"string","required":true,"enum":["pdf","png","jpg","mp4","gif"]},{"name":"output_path","type":"string"}]},
    {"name":"canva_sql","summary":"Read-only SQL against local cache","args":[{"name":"query","type":"string","required":true},{"name":"limit","type":"number","default":500,"max":10000}]},
    {"name":"canva_schema","summary":"Return this schema","args":[{"name":"mode","type":"string","enum":["compact","full"],"default":"compact"}]}
  ],
  "global_flags": ["json","no-cache","quiet","auto-wait"],
  "exit_codes": {"0":"success","2":"auth_required/auth_revoked","3":"not_found","4":"network","5":"validation","6":"rate_limited","7":"permission_denied"},
  "error_envelope": {"error":"<stable code>","message":"<human>","fix":"<literal command to retry>","exit_code":1}
}`
