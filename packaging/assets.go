package packaging

import _ "embed"

var (
	//go:embed config/vreflink.env
	guestConfigTemplate string

	//go:embed deb/control.template
	debControlTemplate string

	//go:embed systemd/vreflinkd.service
	systemdUnitTemplate string

	//go:embed systemd/vreflinkd.env
	daemonDefaultsTemplate string
)

func GuestConfigTemplate() []byte {
	return []byte(guestConfigTemplate)
}

func DebControlTemplate() []byte {
	return []byte(debControlTemplate)
}

func SystemdUnitTemplate() []byte {
	return []byte(systemdUnitTemplate)
}

func DaemonDefaultsTemplate() []byte {
	return []byte(daemonDefaultsTemplate)
}
