Simple mini-framework to create command line applications in Slack.

# Usage
```go
package main

import (
   "github.com/mtyurt/slackcommander"
)

func main() {
	mux := &slackcommander.SlackMux{}
	mux.Token = "commaseparatedslacktokenshere"
	helpCommand := slackcommander.CommandDef{Handler: func(args slackcommander.CommandArgs) (*slackcommander.CommandResponse, error) {
		resp := slackcommander.SimpleTextResponse("Hello, " + args.User)
		return &resp, nil

	}}
	mux.RegisterCommandHandler("help", helpCommand)
	http.HandleFunc("/", mux.SlackHandler())
	http.ListenAndServe(":8080", nil)
}
```

You can register commands like this, this mini-framework handles HTTP request parsing, command matching, invalid commands, and preparing response format with returned string.
