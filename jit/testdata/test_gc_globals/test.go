package test_gc_globals

import (
	"github.com/ringsaturn/tzf"
)

var finder, err = tzf.NewDefaultFinder()

func Find(lat, lon float64) string {
	return finder.GetTimezoneName(lat, lon)
}
