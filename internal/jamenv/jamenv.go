package jamenv

import (
	"os"
)

type JamEnv int

const (
	Prod JamEnv = iota
	Dev
	Local
)

func (e JamEnv) String() string {
	switch e {
	case Prod:
		return "prod"
	case Dev:
		return "dev"
	case Local:
		return "local"
	}
	return "unknown"
}

func Env() JamEnv {
	jamEnvString := os.Getenv("JAM_ENV")
	switch jamEnvString {
	case "dev":
		return Dev
	case "local":
		return Local
	default:
		return Prod
	}
}

func PublicAPIAddress() string {
	if Env() != Prod {
		return os.Getenv("JAM_SERVER_IP")
	}
	return "18.188.17.102:14357"
}

func Auth0ClientID() string {
	if Env() != Prod {
		return os.Getenv("AUTH0_CLI_CLIENT_ID")
	}
	return "RtU3pgK8TjA21ovnPmdPiSMPNY7PHTER"
}

func Auth0Domain() string {
	if Env() != Prod {
		return os.Getenv("AUTH0_DOMAIN")
	}
	return "jamsync.us.auth0.com"
}
