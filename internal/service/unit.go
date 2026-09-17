package service

import "strings"

// renderSystemdUnit renders the systemd --user unit installed on Linux.
func renderSystemdUnit(spec Spec) string {
	var b strings.Builder
	b.WriteString("[Unit]\n")
	b.WriteString("Description=sandwarden convergence loop (sandwarden watch)\n")
	b.WriteString("Documentation=https://github.com/JLugagne/sandwarden/blob/main/docs/service.md\n")
	b.WriteString("\n[Service]\n")
	b.WriteString("Type=simple\n")
	b.WriteString("ExecStart=" + systemdCommandLine(spec) + "\n")
	b.WriteString("Restart=on-failure\n")
	b.WriteString("RestartSec=5\n")
	b.WriteString("\n[Install]\n")
	b.WriteString("WantedBy=default.target\n")
	return b.String()
}

func systemdCommandLine(spec Spec) string {
	parts := make([]string, 0, len(spec.Args)+1)
	parts = append(parts, systemdEscape(spec.Executable))
	for _, arg := range spec.Args {
		parts = append(parts, systemdEscape(arg))
	}
	return strings.Join(parts, " ")
}

// systemdEscape quotes one ExecStart argument for systemd: arguments with
// whitespace or quotes are double-quoted, and "%" is doubled because systemd
// expands specifiers in unit files.
func systemdEscape(arg string) string {
	arg = strings.ReplaceAll(arg, "%", "%%")
	if arg == "" {
		return `""`
	}
	if !strings.ContainsAny(arg, " \t\"'\\") {
		return arg
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(arg) + `"`
}

// renderLaunchdPlist renders the launchd agent installed on macOS.
func renderLaunchdPlist(spec Spec) string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n")
	b.WriteString("<plist version=\"1.0\">\n")
	b.WriteString("<dict>\n")
	b.WriteString("\t<key>Label</key>\n")
	b.WriteString("\t<string>" + xmlEscape(LaunchdLabel) + "</string>\n")
	b.WriteString("\t<key>ProgramArguments</key>\n")
	b.WriteString("\t<array>\n")
	b.WriteString("\t\t<string>" + xmlEscape(spec.Executable) + "</string>\n")
	for _, arg := range spec.Args {
		b.WriteString("\t\t<string>" + xmlEscape(arg) + "</string>\n")
	}
	b.WriteString("\t</array>\n")
	b.WriteString("\t<key>RunAtLoad</key>\n")
	b.WriteString("\t<true/>\n")
	b.WriteString("\t<key>KeepAlive</key>\n")
	b.WriteString("\t<true/>\n")
	b.WriteString("</dict>\n")
	b.WriteString("</plist>\n")
	return b.String()
}

func xmlEscape(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	).Replace(s)
}
