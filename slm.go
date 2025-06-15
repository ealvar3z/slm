// slm.go (small language model)
// slm: a small stub port of Simon W's llm cli for 9front
// Uses ndb for history with Plan 9 style naming.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mischief/ndb"
)

const (
	HISTDIR  = "lib/llm"
	HISTFILE = "llm.history"
	APIURL   = "https://api.openai.com/v1/chat/completions"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Choice struct {
	Message Message `json:"message"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Temperature float64   `json:"temperature"`
	Messages    []Message `json:"messages"`
}

type ChatResponse struct {
	Choices []Choice `json:"choices"`
}

type Opts struct {
	Model      string
	Temp       float64
	SysPrompt  string
	UserPrompt string
	Continue   bool
	APIKey     string
	Home       string
}

type CLIError struct {
	Context 	string
	Err		error
}

func (e CLIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Context, e.Err)
	}
	return e.Context
}

func wrap(context string, e error) error {
	return CLIError{Context: context, Err: e}
}

func logit(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Fatal(wrap(msg, nil))
}

func checkit(err error, context string) {
	if err != nil {
		wrap(context, err)
	}
}

func main() {
	opts := parseflags()
	ensurehistdir(opts.Home)

	msgs := []Message{}
	if opts.Continue {
		msgs = loadhist(opts.Home)
	}
	if opts.SysPrompt != "" {
		msgs = append(msgs, Message{"system", opts.SysPrompt})
	}
	msgs = append(msgs, Message{"user", opts.UserPrompt})

	reply, err := sendchat(opts, msgs)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(reply)

	if opts.Continue {
		appendhist(opts.Home, opts.UserPrompt, reply)
	}
}

func parseflags() *Opts {
	model := flag.String("m", "gpt-3.5-turbo", "model to use")
	temp  := flag.Float64("t", 0.7, "temperature")
	sysp := flag.String("s", "", "system prompt")
	cont := flag.Bool("c", false, "continue with history via NDB")
	flag.Parse()

	apikey := os.Getenv("OPENAI_API_KEY")
	if apikey == "" {
		logit("[ERROR]: OPENAI_API_KEY not set")
	}

	var userp string
	if flag.NArg() > 0 {
		userp = flag.Arg(0)
	} else {
		data, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			checkit(err, "[ERROR]: prompt could not be read")
			userp = string(data)
		}
	}

	home := os.Getenv("home")
	if home == "" {
		home = os.Getenv("HOME")
	}

	return &Opts{
		Model:      *model,
		Temp:       *temp,
		SysPrompt:  *sysp,
		UserPrompt: userp,
		Continue:   *cont,
		APIKey:     apikey,
		Home:       home,
	}
}

func ensurehistdir(home string) {
	dir := filepath.Join(home, HISTDIR)
	if err := os.MkdirAll(dir, 0755); err != nil {
		checkit(err, "[ERROR]: creating history dir:")
	}
}

func histpath(home string) string {
	return filepath.Join(home, HISTDIR, HISTFILE)
}

func loadhist(home string) []Message {
	path := histpath(home)
	db, err := ndb.Open(path)
	if err != nil {
		checkit(err, "no history file or ndb parse error")
	}
	recs := db.Search("role", "")
	msgs := make([]Message, 0, len(recs))
	for _, rec := range recs {
		var role, content string
		for _, tuple := range rec {
			if tuple.Attr == "role" {
				role = tuple.Val
			}
			if tuple.Attr == "content" {
				content = tuple.Val
			}
		}
		if role != "" && content != "" {
			msgs = append(msgs, Message{Role: role, Content: content})
		}
	}
	return msgs
}

func appendhist(home, userp, reply string) {
	path := histpath(home)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		logit("[ERROR] open history src: ", err)
	}
	defer f.Close()

	fmt.Fprintf(f, "message role=%q content=%q\n", "user", userp)
	fmt.Fprintf(f, "message role=%q content=%q\n", "assistant", reply)
}

func sendchat(opts *Opts, msgs []Message) (string, error) {
	req := ChatRequest{Model: opts.Model, Temperature: opts.Temp, Messages: msgs}
	buf, err := json.Marshal(req)
	if err != nil {
		return "", wrap("[ERROR]: marshalling request: ", err)
	}

	reqhttp, err := http.NewRequest("POST", APIURL, bytes.NewReader(buf))
	if err != nil {
		return "", wrap("[ERROR]: creating request: ", err)
	}
	reqhttp.Header.Set("Content-Type", "application/json")
	reqhttp.Header.Set("Authorization", "Bearer "+opts.APIKey)

	resp, err := http.DefaultClient.Do(reqhttp)
	if err != nil {
		return "", wrap("[ERROR]: request error: ", err)
	}
	defer resp.Body.Close()

	var cres ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cres); err != nil {
		return "", wrap("[ERROR]: decode response: ", err)
	}
	if len(cres.Choices) == 0 {
		return "", wrap("[ERROR]: no choices in response", nil)
	}
	return cres.Choices[0].Message.Content, nil
}



