package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Size encodes the physical footprint of a container. ISO sizes are expressed
// in feet and drive slot occupancy in a bay: a 40-foot container consumes two
// TEU slots while a 20-foot container consumes one.
type Size string

const (
	Size20 Size = "20ft"
	Size40 Size = "40ft"
	Size45 Size = "45ft"
)

// TEU returns the twenty-foot-equivalent units a size occupies in a bay row.
func (s Size) TEU() int {
	switch s {
	case Size20:
		return 1
	case Size40, Size45:
		return 2
	default:
		return 0
	}
}

func (s Size) Valid() bool { return s == Size20 || s == Size40 || s == Size45 }

// CargoClass captures the handling hazards associated with the goods inside a
// container. It is consulted by the stacking policy: dangerous goods must be
// isolated from one another, while reefer containers require powered slots.
type CargoClass string

const (
	CargoGeneral   CargoClass = "general"
	CargoDangerous CargoClass = "dangerous"
	CargoReefer    CargoClass = "reefer"
	CargoOversize  CargoClass = "oversize"
)

func (c CargoClass) Valid() bool {
	switch c {
	case CargoGeneral, CargoDangerous, CargoReefer, CargoOversize:
		return true
	}
	return false
}

// DangerProfile refines a dangerous-goods declaration with the UN hazard class
// and an IMO segregation group. Two dangerous containers may only share a bay
// when their segregation groups are compatible; otherwise they must be kept on
// separate rows at least one slot apart.
type DangerProfile struct {
	UNClass          string `json:"un_class"`
	SegregationGroup string `json:"segregation_group"`
}

// Compatible reports whether two dangerous profiles may be stowed near each
// other. Same-group cargoes are treated as incompatible to force physical
// separation; different groups are allowed to coexist in adjacent slots.
func (p DangerProfile) Compatible(other DangerProfile) bool {
	if p.UNClass == "" || other.UNClass == "" {
		return true
	}
	if p.SegregationGroup == "" || other.SegregationGroup == "" {
		return true
	}
	return p.SegregationGroup != other.SegregationGroup
}

// Container is the core aggregate of the yard: a physical box identified by its
// ISO BIC code, with handling attributes that constrain where it may be stacked.
type Container struct {
	ID            string        `json:"id"`
	BIC           string        `json:"bic"`
	Size          Size          `json:"size"`
	CargoClass    CargoClass    `json:"cargo_class"`
	Danger        DangerProfile `json:"danger,omitempty"`
	GrossWeightKg int           `json:"gross_weight_kg"`
	Owner         string        `json:"owner"`
	VoyageIn      string        `json:"voyage_in"`
	VoyageOut     string        `json:"voyage_out"`
	NeedsPower    bool          `json:"needs_power"`
	CreatedAt     time.Time     `json:"created_at"`
}

// Validate checks that the container profile is internally consistent: the
// identifiers are present, the size and cargo class are known, dangerous goods
// carry a UN class, and the weight is within the range a yard crane may lift.
func (c *Container) Validate() error {
	c.ID = strings.ToUpper(strings.TrimSpace(c.ID))
	c.BIC = strings.ToUpper(strings.TrimSpace(c.BIC))
	c.Owner = strings.TrimSpace(c.Owner)
	if c.ID == "" {
		return fmt.Errorf("container id is required")
	}
	if c.BIC == "" {
		return fmt.Errorf("container BIC code is required")
	}
	if !c.Size.Valid() {
		return fmt.Errorf("unknown container size %q", c.Size)
	}
	if !c.CargoClass.Valid() {
		return fmt.Errorf("unknown cargo class %q", c.CargoClass)
	}
	if c.CargoClass == CargoDangerous {
		if c.Danger.UNClass == "" {
			return fmt.Errorf("dangerous goods require a UN class")
		}
		if c.Danger.SegregationGroup == "" {
			c.Danger.SegregationGroup = c.Danger.UNClass
		}
	}
	if c.CargoClass == CargoReefer {
		c.NeedsPower = true
	}
	if c.GrossWeightKg < 0 || c.GrossWeightKg > 34000 {
		return fmt.Errorf("gross weight %d out of handling range", c.GrossWeightKg)
	}
	if c.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}
	return nil
}

// RequiresPower returns true when the container must occupy a powered slot.
func (c Container) RequiresPower() bool { return c.NeedsPower || c.CargoClass == CargoReefer }

// IsDangerous returns true when the container carries regulated dangerous goods.
func (c Container) IsDangerous() bool { return c.CargoClass == CargoDangerous }

// NormalizeTags is retained for callers that attach free-form labels to a
// container record; it lower-cases, trims, de-duplicates and sorts them.
func NormalizeTags(tags []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
