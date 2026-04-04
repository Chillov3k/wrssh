package observers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/NHAS/reverse_ssh/pkg/observer"
)

type AdminSessionState struct {
	Status      string
	SessionID   string
	Username    string
	RemoteAddr  string
	Privilege   string
	Timestamp   time.Time
	Source      string
	Description string
}

func (as AdminSessionState) Summary() string {
	return fmt.Sprintf("%s %s %s %s", as.Username, as.RemoteAddr, as.Status, as.Source)
}

func (as AdminSessionState) Json() ([]byte, error) {
	return json.Marshal(as)
}

var AdminConnectionState = observer.New[AdminSessionState]()
