package ossec

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"crypto/md5"
	"fmt"
)

type FimData struct {
	ID       int64  `json:"id,omitempty"`
	Begin    string `json:"begin"`
	End      string `json:"end"`
	Checksum string `json:"checksum"`
}
type FimMessage struct {
	Component string  `json:"component,omitempty"`
	Type      string  `json:"type"`
	Data      FimData `json:"data"`
}

func NewFimMessage() (*FimMessage, error) {
	filename := filepath.Base(os.Args[0])
	hasher := md5.New()
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := io.Copy(hasher, f); err != nil {
		return nil, err
	}

	hash := fmt.Sprintf("%x", hasher.Sum(nil))

	return &FimMessage{
		Component: "fim_file",
		Type:      "integrity_check_global",
		Data: FimData{
			ID:       time.Now().Unix(),
			Begin:    filename,
			End:      filename,
			Checksum: hash,
		},
	}, nil
}

// Send Integrity Status
func (a *Client) ReportIntegrity() error {
	a.mx.Lock()
	defer a.mx.Unlock()
	return a.reportIntegrity()
}

func (a *Client) reportIntegrity() error {
	finMsg, err := NewFimMessage()
	if err != nil {
		return err
	}
	b, err := json.Marshal(finMsg)
	if err != nil {
		return err
	}

	msg := fmt.Sprintf("%c:%s:%s", SYSCHECK_MQ, HC_SK, string(b))

	err = a.writeMessage(msg)
	if err != nil {
		return err
	}

	return nil

}
