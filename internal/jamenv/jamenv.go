package jamenv

import "os"

type JamEnv int

const (
	Prod JamEnv = iota
	Dev
	Local
	Memory
)

func (e JamEnv) String() string {
	switch e {
	case Prod:
		return "prod"
	case Dev:
		return "dev"
	case Local:
		return "local"
	case Memory:
		return "memory"
	}
	return "unknown"
}

func Env() JamEnv {
	jamEnvString := os.Getenv("JAMENV")
	switch jamEnvString {
	case "prod":
		return Prod
	case "dev":
		return Dev
	case "local":
		return Local
	case "memory":
		return Memory
	}
	panic("invalid JAMENV environment variable")
}

var LocalAPIAddress = "0.0.0.0:14357"

func PublicAPIAddress() string {
	switch Env() {
	case Prod:
		return "18.188.17.102:14357"
		//return "jamsync.dev:14357"
	case Dev:
		return "TODO"
	case Local:
		return LocalAPIAddress
	}
	panic("could not get server address for JAMENV " + Env().String())
}

func Auth0ClientID() string {
	switch Env() {
	case Prod:
		return "287nBofX8C9oAm08ysKXcms0PKf9lns7"
	case Local, Dev:
		return "pEBqAnFPPaONbdST1zuXlxlmZjsnfysr"
	}
	panic("could not get auth0 client id for JAMENV " + Env().String())
}

func Auth0Domain() string {
	switch Env() {
	case Prod:
		return "jamsync.us.auth0.com"
	case Local, Dev:
		return "dev-dzb-qyan.us.auth0.com"
	}
	panic("could not get auth0 domain for JAMENV " + Env().String())
}

func Auth0RedirectUrl() string {
	switch Env() {
	case Prod:
		return "jamsync.dev"
	case Local, Dev:
		return "http://localhost:8082/callback"
	}
	panic("could not get auth0 redirect url for JAMENV " + Env().String())
}

func UseAuth() bool {
	switch Env() {
	case Prod, Local, Dev:
		return true
	case Memory:
		return false
	}
	panic("could not get auth value for JAMENV " + Env().String())
}
