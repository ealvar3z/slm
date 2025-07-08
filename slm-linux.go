// slm_linux.go
// slm: a small stub CLI for OpenAI GPT on Linux
// Uses ndb for history stored in $XDG_CONFIG_HOME/slm/history.ndb
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
	AppName  = "slm"
	HistFile = "history.ndb"
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
}

func main() {
	opts := parseFlags()
	if err := ensureHistDir(); err != nil {
		log.Fatal(err)
	}

	var msgs []Message
	if opts.Continue {
		msgs = loadHist()
	}
	if opts.SysPrompt != "" {
		msgs = append(msgs, Message{"system", opts.SysPrompt})
	}
	msgs = append(msgs, Message{"user", opts.UserPrompt})

	reply, err := sendChat(opts, msgs)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(reply)

	if opts.Continue {
		appendHist(opts.UserPrompt, reply)
	}
}

func parseFlags() *Opts {
	model := flag.String("m", "gpt-3.5-turbo", "model to use")
	temp := flag.Float64("t", 0.7, "temperature")
	sysp := flag.String("s", "", "system prompt")
	cont := flag.Bool("c", false, "continue with history via NDB")
	flag.Parse()

	apikey := os.Getenv("OPENAI_API_KEY")
	if apikey == "" {
		log.Fatal("[ERROR] OPENAI_API_KEY not set")
	}

	var userp string
	if flag.NArg() > 0 {
		userp = flag.Arg(0)
	} else {
		data, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			log.Fatalf("[ERROR] prompt could not be read: %v", err)
		}
		userp = string(data)
	}

	return &Opts{
		Model:      *model,
		Temp:       *temp,
		SysPrompt:  *sysp,
		UserPrompt: userp,
		Continue:   *cont,
		APIKey:     apikey,
	}
}

func histDir() string {
	confDir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		confDir = filepath.Join(home, ".config")
	}
	return filepath.Join(confDir, AppName)
}

func histPath() string {
	return filepath.Join(histDir(), HistFile)
}

func ensureHistDir() error {
	return os.MkdirAll(histDir(), 0o755)
}

func loadHist() []Message {
	db, err := ndb.Open(histPath())
	if err != nil {
		return nil
	}
	recs := db.Search("role", "")
	msgs := make([]Message, 0, len(recs))
	for _, rec := range recs {
		var role, content string
		for _, tup := range rec {
			if tup.Attr == "role" {
				role = tup.Val
			}
			if tup.Attr == "content" {
				content = tup.Val
			}
		}
		if role != "" && content != "" {
			msgs = append(msgs, Message{Role: role, Content: content})
		}
	}
	return msgs
}

func appendHist(userp, reply string) {
	f, err := os.OpenFile(histPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatalf("[ERROR] opening history file: %v", err)
	}
	defer f.Close()

	fmt.Fprintf(f, "message role=%q content=%q\n", "user", userp)
	fmt.Fprintf(f, "message role=%q content=%q\n", "assistant", reply)
}

func sendChat(opts *Opts, msgs []Message) (string, error) {
	reqBody := ChatRequest{Model: opts.Model, Temperature: opts.Temp, Messages: msgs}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("[ERROR] marshalling request: %w", err)
	}

	req, err := http.NewRequest("POST", APIURL, bytes.NewReader(buf))
	if err != nil {
		return "", fmt.Errorf("[ERROR] creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opts.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("[ERROR] request error: %w", err)
	}
	defer resp.Body.Close()

	// Read full body for error handling and parsing
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("[ERROR] reading response body: %w", err)
	}

	// Handle HTTP errors
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API error: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	// Check for API-level errors in JSON
	var errResp struct {
		Error struct { Message string `json:"message"` } `json:"error"`
	}
	if err := json.Unmarshal(bodyBytes, &errResp); err == nil && errResp.Error.Message != "" {
		return "", fmt.Errorf("OpenAI API error: %s", errResp.Error.Message)
	}

	// Parse successful response
	var cres ChatResponse
	if err := json.Unmarshal(bodyBytes, &cres); err != nil {
		return "", fmt.Errorf("[ERROR] decoding response: %w", err)
	}
	if len(cres.Choices) == 0 {
		return "", fmt.Errorf("[ERROR] no choices in response")
	}
	return cres.Choices[0].Message.Content, nil
}

