package webui

// statsSaldoHue returns the bare hue token for a saldo value:
// green when ahead (pos=true), red when behind.
func statsSaldoHue(pos bool) string {
	if pos {
		return "green"
	}
	return "red"
}
