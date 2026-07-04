package webui

// LogoShape decides the hero treatment for an uploaded logo: near-square
// images get the Kristall hexagon crop, wide/tall wordmarks render intact on
// a glass tile (contain). Unmeasured legacy rows (0×0) keep the hex crop.
func LogoShape(w, h int) string {
	if w <= 0 || h <= 0 {
		return "hex"
	}
	r := float64(w) / float64(h)
	if r >= 0.8 && r <= 1.25 {
		return "hex"
	}
	return "tile"
}
