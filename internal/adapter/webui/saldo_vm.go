package webui

// statsSaldoHue returns the Tailwind color class for a saldo value:
// green when ahead (pos=true), red when behind.
func statsSaldoHue(pos bool) string {
	if pos {
		return "text-green"
	}
	return "text-red"
}
