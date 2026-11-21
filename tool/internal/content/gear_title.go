package content

import "strings"

// corporateWords are dropped from camera names ("NIKON CORPORATION ...").
var corporateWords = map[string]bool{
	"CORPORATION": true, "CORP": true, "CO": true, "LTD": true, "INC": true, "CO.,LTD": true,
	"COMPANY": true, "AG": true, "GMBH": true,
}

// descriptiveWords ("RICOH IMAGING COMPANY", "Leica Camera AG") are dropped
// only when a corporate word follows them.
var descriptiveWords = map[string]bool{"IMAGING": true, "CAMERA": true}

// brands maps an upper-case brand to its usual spelling. Brands not listed
// (e.g. ZWO, DJI) keep the spelling they have in EXIF.
var brands = map[string]string{}

func init() {
	for _, b := range []string{"Nikon", "Sony", "Canon", "Fujifilm", "Panasonic",
		"Olympus", "Pentax", "Leica", "Ricoh", "Sigma", "Hasselblad"} {
		brands[strings.ToUpper(b)] = b
	}
}

func isCorporate(tok string) bool {
	return corporateWords[strings.TrimRight(strings.ToUpper(tok), ".,")]
}

// GearTitle returns the display name for a gear value from EXIF. Camera names
// (EXIF Make + " " + Model) lose corporate words and a repeated make, and a
// known brand gets its usual spelling: "NIKON CORPORATION NIKON Z 8" becomes
// "Nikon Z 8". Lens names, and any other gearType, are only trimmed.
func GearTitle(raw, gearType string) string {
	if gearType != "camera" {
		return strings.TrimSpace(raw)
	}
	in := strings.Fields(raw)
	var toks []string
	for i, tok := range in {
		if isCorporate(tok) {
			continue
		}
		if descriptiveWords[strings.ToUpper(tok)] && i+1 < len(in) && isCorporate(in[i+1]) {
			continue
		}
		toks = append(toks, tok)
	}
	if len(toks) > 1 {
		if strings.EqualFold(toks[0], toks[1]) {
			toks = append(toks[:1], toks[2:]...)
		} else if _, ok := brands[strings.ToUpper(toks[1])]; ok {
			// A brand made by another company ("RICOH ... PENTAX K-3"): the
			// model's own brand wins.
			toks = toks[1:]
		}
	}
	if len(toks) > 0 {
		if b, ok := brands[strings.ToUpper(toks[0])]; ok {
			toks[0] = b
		}
	}
	return strings.Join(toks, " ")
}
