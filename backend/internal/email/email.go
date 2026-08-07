package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"

	"github.com/R7rainz/switchyard/backend/internal/credential"
	"github.com/R7rainz/switchyard/backend/internal/execution"
)

const credentialProvider = "email"
const credentialName = "default"

var ErrNoCredential = errors.New("email: missing credential email/default")

type Credentials interface {
	Get(context.Context, string, string, string) (credential.Secret, error)
}

type messageRunner struct{ credentials Credentials }

type smtpConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

func Runners(credentials Credentials) execution.Registry {
	return execution.Registry{"email.message": &messageRunner{credentials: credentials}}
}

func (e *messageRunner) Run(ctx context.Context, in execution.Input) (execution.Result, error) {
	var data map[string]string
	if err := json.Unmarshal(in.Data, &data); err != nil {
		return execution.Result{}, fmt.Errorf("email message data: %w", err)
	}
	to, subject, text := strings.TrimSpace(data["to"]), strings.TrimSpace(data["subject"]), data["text"]
	if to == "" || subject == "" || text == "" {
		return execution.Result{}, errors.New("email message needs to, subject, and text")
	}
	if strings.ContainsAny(to+subject, "\r\n") {
		return execution.Result{}, errors.New("email message headers must not contain newlines")
	}
	secret, err := e.credentials.Get(ctx, in.WorkspaceID, credentialProvider, credentialName)
	if errors.Is(err, credential.ErrNotFound) {
		return execution.Result{}, ErrNoCredential
	}
	if err != nil {
		return execution.Result{}, fmt.Errorf("email credential: %w", err)
	}
	var config smtpConfig
	if err := json.Unmarshal(secret, &config); err != nil {
		return execution.Result{}, errors.New("email: credential email/default must be SMTP JSON")
	}
	if config.Host == "" || config.From == "" || strings.ContainsAny(config.From, "\r\n") {
		return execution.Result{}, errors.New("email: SMTP credential needs host and from")
	}
	if config.Port == 0 {
		config.Port = 587
	}
	host, _, err := net.SplitHostPort(net.JoinHostPort(config.Host, strconv.Itoa(config.Port)))
	if err != nil {
		return execution.Result{}, errors.New("email: SMTP host is invalid")
	}
	if err := ctx.Err(); err != nil {
		return execution.Result{}, err
	}
	var auth smtp.Auth
	if config.Username != "" {
		auth = smtp.PlainAuth("", config.Username, config.Password, host)
	}
	message := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", config.From, to, subject, text))
	if err := smtp.SendMail(net.JoinHostPort(config.Host, strconv.Itoa(config.Port)), auth, config.From, []string{to}, message); err != nil {
		return execution.Result{}, fmt.Errorf("email message: %w", err)
	}
	return execution.Result{Output: json.RawMessage("{\"sent\":true}")}, nil
}
