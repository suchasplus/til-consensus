package artifact

import (
	"time"

	"github.com/suchasplus/til-consensus/consensus"
)

var RequestIDPattern = consensus.RequestIDPattern

func NewRequestID(now time.Time) string {
	return consensus.NewRequestID(now)
}
