package peer

import "time"

type Peer struct {
	NodeID    string
	Addr      string
	LastSeenAt time.Time
}
