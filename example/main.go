package main

import (
	"net/http"

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
