package misc

import (
	"fmt"
	"math/rand"
)

// Grundform -> männlich / weiblich
type adjective struct {
	male   string
	female string
}

var adjectives = []adjective{
	{"mutiger", "mutige"}, {"fröhlicher", "fröhliche"}, {"kluger", "kluge"},
	{"ruhiger", "ruhige"}, {"wilder", "wilde"}, {"stolzer", "stolze"},
	{"tapferer", "tapfere"}, {"sanfter", "sanfte"}, {"schneller", "schnelle"},
	{"starker", "starke"}, {"neugieriger", "neugierige"}, {"gelassener", "gelassene"},
	{"pfiffiger", "pfiffige"}, {"munterer", "muntere"}, {"kühner", "kühne"},
}

var maleNames = []string{
	"Anton", "Bruno", "Dieter", "Emil", "Felix", "Gustav",
	"Hans", "Jonas", "Karl", "Ludwig", "Otto", "Theo",
}

var femaleNames = []string{
	"Ada", "Berta", "Clara", "Elsa", "Frida", "Greta",
	"Hanna", "Ida", "Lene", "Martha", "Nora", "Trude",
}

func RandomName() string {
	adj := adjectives[rand.Intn(len(adjectives))]
	if rand.Intn(2) == 0 {
		return fmt.Sprintf("%s_%s", adj.male, maleNames[rand.Intn(len(maleNames))])
	}
	return fmt.Sprintf("%s_%s", adj.female, femaleNames[rand.Intn(len(femaleNames))])
}
