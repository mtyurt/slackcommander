Simple mini-framework to create command line applications in Slack.

# Usage
```
package main

import (
   "github.com/mtyurt/slackcommander"
)

func main() {
    mux := &slackcommander.SlackMux{}
    mux.Token = "slacktokenhere"
    mux.RegisterCommand("help", func(user string, args []string) (string, error) {
        return "Hello," + user, nil
    })
    http.HandleFunc("/", mux.SlackHandler())
    http.ListenAndServe(":8080", nil)
}
```

You can register commands like this, this mini-framework handles HTTP request parsing, command matching, invalid commands, and preparing response format with returned string.
