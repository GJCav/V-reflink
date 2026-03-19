package packaging

import _ "embed"

var (
	//go:embed deb/conffiles
	debConffiles string

	//go:embed config/vreflink.toml
	guestConfigTemplate string

	//go:embed deb/control.template
	debControlTemplate string

	//go:embed deb/postrm
	debPostrm string

	//go:embed deb/prerm
	debPrerm string

	//go:embed systemd/vreflinkd.service
	systemdUnitTemplate string

	//go:embed systemd/vreflinkd.toml
	daemonConfigTemplate string
)

func DebConffiles() []byte {
	return []byte(debConffiles)
}

func GuestConfigTemplate() []byte {
	return []byte(guestConfigTemplate)
}

func DebControlTemplate() []byte {
	return []byte(debControlTemplate)
}

func DebPostrm() []byte {
	return []byte(debPostrm)
}

func DebPrerm() []byte {
	return []byte(debPrerm)
}

func SystemdUnitTemplate() []byte {
	return []byte(systemdUnitTemplate)
}

func DaemonConfigTemplate() []byte {
	return []byte(daemonConfigTemplate)
}
