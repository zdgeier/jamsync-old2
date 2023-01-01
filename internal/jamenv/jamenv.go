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

func PublicAPIAddress() string {
	switch Env() {
	case Prod:
		return "18.188.17.102:14357"
		//return "jamsync.dev:14357"
	case Dev:
		return "TODO"
	case Local:
		return "0.0.0.0:14357"
	}
	panic("could not get server address for JAMENV " + Env().String())
}
