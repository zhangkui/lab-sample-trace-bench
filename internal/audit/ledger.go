// Package audit implements the tamper-evident ledger of yard operations.
//
// Every state-changing operation appends an entry that chains to the previous
// one via a running checksum: each entry's checksum covers the entry's own
// fields concatenated with the prior checksum. This makes any after-the-fact
// edit or deletion detectable, because the chain breaks at the first altered
// entry. The yard service validates the chain on demand and refuses to operate
// when integrity is broken.
package audit

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"sort"
	"strings"
	"time"
)

// Entry is one record in the audit chain. Subject is the id of the container,
// task or anomaly the entry concerns; Action is the verb performed; Detail is a
// free-form JSON string with the operation payload for later reconstruction.
type Entry struct {
	Seq      int64  `json:"seq"`
	Subject  string `json:"subject"`
	Action   string `json:"action"`
	Actor    string `json:"actor"`
	Detail   string `json:"detail"`
	Checksum uint32 `json:"checksum"`
	At       string `json:"at"`
}

// Ledger persists audit entries in a sqlite table provided by the store.
type Ledger struct{ db *sql.DB }

// New returns a Ledger backed by db. The audit table is created if absent.
func New(db *sql.DB) (*Ledger, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS audit (
		seq INTEGER PRIMARY KEY AUTOINCREMENT,
		subject TEXT NOT NULL,
		action TEXT NOT NULL,
		actor TEXT NOT NULL,
		detail TEXT NOT NULL DEFAULT '',
		checksum INTEGER NOT NULL DEFAULT 0,
		at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_audit_subject ON audit(subject);`)
	if err != nil {
		return nil, err
	}
	return &Ledger{db: db}, nil
}

// Append records a new entry, chaining its checksum to the previous entry.
// The detail argument is serialised to canonical JSON so that byte-identical
// payloads produce identical checksums regardless of map ordering.
func (l *Ledger) Append(subject, action, actor string, detail any) (Entry, error) {
	if strings.TrimSpace(subject) == "" || strings.TrimSpace(action) == "" {
		return Entry{}, fmt.Errorf("audit subject and action are required")
	}
	body, err := canonical(detail)
	if err != nil {
		return Entry{}, err
	}
	prev, err := l.lastChecksum()
	if err != nil {
		return Entry{}, err
	}
	at := time.Now().UTC().Format(time.RFC3339Nano)
	sum := chain(subject, action, actor, body, prev, at)
	res, err := l.db.Exec(`INSERT INTO audit(subject,action,actor,detail,checksum,at)
VALUES(?,?,?,?,?,?)`, subject, action, actor, body, sum, at)
	if err != nil {
		return Entry{}, err
	}
	seq, _ := res.LastInsertId()
	return Entry{Seq: seq, Subject: subject, Action: action, Actor: actor, Detail: body, Checksum: sum, At: at}, nil
}

// Verify walks the whole chain and confirms that every checksum still matches.
// It returns the index (1-based) of the first broken entry, or 0 when the
// chain is intact. An intact chain proves no entry was edited or removed.
func (l *Ledger) Verify() (int, error) {
	rows, err := l.db.Query(`SELECT seq,subject,action,actor,detail,checksum,at FROM audit ORDER BY seq`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var prev uint32
	idx := 0
	for rows.Next() {
		idx++
		var e Entry
		if err := rows.Scan(&e.Seq, &e.Subject, &e.Action, &e.Actor, &e.Detail, &e.Checksum, &e.At); err != nil {
			return idx, err
		}
		if e.Checksum != chain(e.Subject, e.Action, e.Actor, e.Detail, prev, e.At) {
			return idx, nil
		}
		prev = e.Checksum
	}
	return 0, rows.Err()
}

// History returns the audit chain for a subject, ordered by sequence.
func (l *Ledger) History(subject string) ([]Entry, error) {
	rows, err := l.db.Query(`SELECT seq,subject,action,actor,detail,checksum,at
FROM audit WHERE subject=? ORDER BY seq`, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Seq, &e.Subject, &e.Action, &e.Actor, &e.Detail, &e.Checksum, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// All returns the complete chain for whole-store integrity checks.
func (l *Ledger) All() ([]Entry, error) {
	rows, err := l.db.Query(`SELECT seq,subject,action,actor,detail,checksum,at FROM audit ORDER BY seq`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Seq, &e.Subject, &e.Action, &e.Actor, &e.Detail, &e.Checksum, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// IntegrityError is returned when the chain is broken.
var IntegrityError = errors.New("audit chain integrity broken")

// MustIntact panics if the chain fails verification. It is a convenience for
// boot-time checks; runtime callers should use Verify directly.
func (l *Ledger) MustIntact() {
	if idx, err := l.Verify(); err != nil || idx != 0 {
		panic(fmt.Sprintf("%v at entry %d", err, idx))
	}
}

// lastChecksum returns the checksum of the most recent entry, or 0 when the
// chain is empty (the genesis entry chains to 0).
func (l *Ledger) lastChecksum() (uint32, error) {
	var sum uint32
	err := l.db.QueryRow(`SELECT checksum FROM audit ORDER BY seq DESC LIMIT 1`).Scan(&sum)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return sum, err
}

// chain computes the checksum of an entry as crc32 over the concatenation of
// the previous checksum and the entry's own fields. Including the previous
// checksum is what makes the ledger a chain rather than independent records.
func chain(subject, action, actor, detail string, prev uint32, at string) uint32 {
	h := crc32.NewIEEE()
	fmt.Fprintf(h, "%d|%s|%s|%s|%s|%s", prev, subject, action, actor, detail, at)
	return h.Sum32()
}

// canonical renders a value as sorted-key JSON so semantically identical
// payloads hash identically. Strings are returned verbatim so callers may pass
// a pre-serialised body.
func canonical(v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	// Re-marshal with sorted keys for determinism.
	var any map[string]any
	if err := json.Unmarshal(raw, &any); err == nil {
		raw, err = json.Marshal(sortedMap(any))
		if err != nil {
			return "", err
		}
	}
	return string(raw), nil
}

func sortedMap(m map[string]any) map[string]any {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		out[k] = m[k]
	}
	return out
}
