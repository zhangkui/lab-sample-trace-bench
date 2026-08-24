package domain

import (
	"fmt"
	"strings"
)

// Zone divides the yard into logical regions, each with its own bay grid and
// handling characteristics. A reefer zone is wired for powered containers, a
// danger zone is earmarked for regulated cargo, and an export/import zone
// groups boxes by the direction of their next move.
type ZoneType string

const (
	ZoneGeneral ZoneType = "general"
	ZoneReefer  ZoneType = "reefer"
	ZoneDanger  ZoneType = "danger"
	ZoneExport  ZoneType = "export"
)

func (z ZoneType) Valid() bool {
	switch z {
	case ZoneGeneral, ZoneReefer, ZoneDanger, ZoneExport:
		return true
	}
	return false
}

// Zone is a top-level region of the yard. It owns a set of bays and tracks the
// aggregate capacity and the classes of cargo it is permitted to receive.
type Zone struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Type        ZoneType `json:"type"`
	BayCount    int      `json:"bay_count"`
	RowsPerBay  int      `json:"rows_per_bay"`
	TiersPerRow int      `json:"tiers_per_row"`
}

// Validate ensures the zone geometry is positive and the type is known.
func (z *Zone) Validate() error {
	z.Code = strings.ToUpper(strings.TrimSpace(z.Code))
	z.Name = strings.TrimSpace(z.Name)
	if z.Code == "" {
		return fmt.Errorf("zone code is required")
	}
	if !z.Type.Valid() {
		return fmt.Errorf("unknown zone type %q", z.Type)
	}
	if z.BayCount < 1 || z.RowsPerBay < 1 || z.TiersPerRow < 1 {
		return fmt.Errorf("zone %s geometry must be positive", z.Code)
	}
	return nil
}

// Permits reports whether a container of the given class may be stowed in this
// zone. Reefer zones accept reefer and general cargo; danger zones accept
// dangerous goods; general/export zones accept everything except dangerous
// goods, which are confined to the danger zone.
func (z Zone) Permits(c CargoClass) bool {
	switch z.Type {
	case ZoneReefer:
		return c == CargoReefer || c == CargoGeneral
	case ZoneDanger:
		return c == CargoDangerous
	case ZoneExport:
		return c != CargoDangerous
	default:
		return c != CargoDangerous
	}
}

// SlotID is the fully-qualified address of a single stacking position within a
// zone: zone-bay-row-tier, e.g. "A-03-02-04". Tiers count from the ground up so
// that tier 1 is the bottom of the stack.
type SlotID string

// NewSlot assembles a slot address from its components after normalising case.
func NewSlot(zone string, bay, row, tier int) SlotID {
	return SlotID(fmt.Sprintf("%s-%02d-%02d-%02d", strings.ToUpper(zone), bay, row, tier))
}

// Parts decomposes a slot address into its zone, bay, row and tier components.
// It returns an error if the address is malformed.
func (s SlotID) Parts() (zone string, bay, row, tier int, err error) {
	var z string
	_, err = fmt.Sscanf(string(s), "%1s-%02d-%02d-%02d", &z, &bay, &row, &tier)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("malformed slot id %q", s)
	}
	return z, bay, row, tier, nil
}

// Ground reports whether the slot sits on the lowest tier.
func (s SlotID) Ground() bool {
	_, _, _, tier, err := s.Parts()
	if err != nil {
		return false
	}
	return tier == 1
}

// Placement captures the state of a single slot: the container occupying it,
// whether the position carries shore power, and the live load it bears.
type Placement struct {
	Slot      SlotID `json:"slot"`
	Container string `json:"container"`
	Powered   bool   `json:"powered"`
}

// Occupancy summarises how full a zone is at a point in time. It is recomputed
// by the yard service after every move so that capacity decisions use fresh
// figures rather than a stale snapshot.
type Occupancy struct {
	Zone       ZoneType `json:"zone_type"`
	Used       int      `json:"used"`
	Capacity   int      `json:"capacity"`
	Dangerous  int      `json:"dangerous"`
	Reefer     int      `json:"reefer"`
	GroundFree int      `json:"ground_free"`
}

// Utilisation returns the fraction of the zone's capacity that is in use.
func (o Occupancy) Utilisation() float64 {
	if o.Capacity == 0 {
		return 0
	}
	return float64(o.Used) / float64(o.Capacity)
}

// Full reports whether the zone can accept no further containers.
func (o Occupancy) Full() bool { return o.Used >= o.Capacity }
