package harness

import "fmt"

type LoopDetect struct {
	lastKey string
	count   int
	limit   int
}

func NewLoopDetect(limit int) *LoopDetect {
	if limit <= 0 {
		limit = 3
	}
	return &LoopDetect{limit: limit}
}

func (d *LoopDetect) Observe(tool, args string) error {
	key := tool + "\x00" + args
	if key == d.lastKey {
		d.count++
	} else {
		d.lastKey = key
		d.count = 1
	}
	if d.count >= d.limit {
		return fmt.Errorf("halted: tool %s repeated %d times with the same arguments", tool, d.count)
	}
	return nil
}
