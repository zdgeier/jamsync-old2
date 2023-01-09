package jamenv

import (
	"os"

	"github.com/joho/godotenv"
)

type JamEnv int

const (
	Prod JamEnv = iota
	Dev
	Local
	Memory
)

func LoadFile() error {
	return godotenv.Load("/etc/jamsync/.env")
}

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
	return os.Getenv("JAM_SERVER_IP")
}

func Auth0ClientID() string {
	return os.Getenv("AUTH0_CLI_CLIENT_ID")
}

func Auth0Domain() string {
	return os.Getenv("AUTH0_DOMAIN")
}
