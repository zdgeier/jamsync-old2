package clientauth

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	cv "github.com/nirasan/go-oauth-pkce-code-verifier"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/viper"
	"github.com/zdgeier/jamsync/internal/jamenv"
)

// TODO: fix error output in this file
var redirectUrl = "http://localhost:8082/callback"

// AuthorizeUser implements the PKCE OAuth2 flow.
func AuthorizeUser() {
	// initialize the code verifier
	var CodeVerifier, _ = cv.CreateCodeVerifier()

	// construct the authorization URL (with Auth0 as the authorization provider)
	authorizationURL := fmt.Sprintf(
		"https://%s/authorize?audience=api.jamsync.dev"+
			"&scope=write:projects"+
			"&response_type=code&client_id=%s"+
			"&code_challenge=%s"+
			"&code_challenge_method=S256&redirect_uri=%s",
		jamenv.Auth0Domain(), jamenv.Auth0ClientID(), CodeVerifier.CodeChallengeS256(), redirectUrl)

	// start a web server to listen on a callback URL
	server := &http.Server{Addr: redirectUrl}

	// define a handler that will get the authorization code, call the token endpoint, and close the HTTP server
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// get the authorization code
		code := r.URL.Query().Get("code")
		if code == "" {
			log.Println("url param 'code' missing")
			io.WriteString(w, "Error: could not find 'code' URL parameter\n")

			// close the HTTP server and return
			cleanup(server)
			return
		}

		// trade the authorization code and the code verifier for an access token
		codeVerifier := CodeVerifier.String()
		token, err := getAccessToken(jamenv.Auth0ClientID(), codeVerifier, code, redirectUrl)
		if err != nil {
			log.Println("could not get access token")
			io.WriteString(w, "Error: could not retrieve access token\n")

			// close the HTTP server and return
			cleanup(server)
			return
		}
		viper.Set("AccessToken", token)
		//_, err = config.WriteConfigFile("auth.json", token)
		if err != nil {
			log.Println("could not write config file")
			io.WriteString(w, "Error: could not store access token\n"+err.Error())

			// close the HTTP server and return
			cleanup(server)
			return
		}

		// return an indication of success to the caller
		io.WriteString(w, `
		<html>
			<head>
			<style>
			@font-face {
				font-display: swap;
				font-family: 'Fira Code';
				font-style: normal;
				font-weight: 400;
				src: url("jamsync.dev/public/FiraCode-Regular.woff2") format("woff2")
			}
	
			@font-face {
				font-display: swap;
				font-family: 'Fira Code';
				font-style: normal;
				font-weight: 800;
				src: url("jamsync.dev/public/FiraCode-Bold.woff2") format("woff2")
			}
			:root {
				--bright-pink: #ff007f;
				--pink: #D24079;
				--purple: rebeccapurple;
				--tan: antiquewhite;
				--dark-blue: #222129;
				background-color: var(--dark-blue);
				color: white;
				margin: 0;
				padding: 0;
				font-family: 'Fira Code', Monaco, Consolas, Ubuntu Mono, monospace;
				font-size: 1rem;
				line-height: 1.54;
				letter-spacing: -0.02em;
				text-rendering: optimizeLegibility;
				-webkit-font-smoothing: antialiased;
				font-feature-settings: "liga", "tnum", "zero", "ss01", "locl";
				font-variant-ligatures: contextual;
				-webkit-overflow-scrolling: touch;
				-webkit-text-size-adjust: 100%;
	
				@media ($phone) {
					font-size: 1rem;
				}
			}
			* {
				box-sizing: border-box;
			}
			html {
				display: flex;
				align-items: center;
				flex-direction: column;
				height: 100%;
			}
			body {
				border-left: 1px solid rgba(255,255,255,0.1);
				border-right: 1px solid rgba(255,255,255,0.1);
				margin: 0;
				width: 100%;
				max-width: 820px;
				display: flex;
				flex-direction: column;
				flex-grow: 1;
			}
			main {
				padding: 32px;
			}
			</style>
			</head>
			<body>
				<main>
					<svg id="a" width="55px" height="55px" version="1.1" viewBox="0 0 256 256" xmlns="http://www.w3.org/2000/svg">
						<g stroke="#000" stroke-linecap="round" stroke-linejoin="round" stroke-miterlimit="2.8">
							<path d="m220.45 14.531c-23.261-1.2646-41.24 16.834-58.42 44.842-1.3476 2.1975-.64013 5.0602 1.58 6.3938 2.22 1.3339 10.692 8.9196 12.04 6.7219 8.1343-25.31 31.513-31.681 51.345-36.795 0 0-2.1438-20.924-6.5446-21.163z" color="#000000" fill="#007a0b" stroke-width="11.269"/>
							<g fill="#ff007f" stroke-width="11.269">
								<ellipse transform="matrix(-.73717 -.67571 .68316 -.73027 0 0)" cx="-189.16" cy="-105.54" rx="36.007" ry="41.807"/>
								<ellipse transform="matrix(-.73717 -.67571 .68316 -.73027 0 0)" cx="-152.84" cy="-68.797" rx="36.007" ry="41.807"/>
								<ellipse transform="matrix(-.73717 -.67571 .68316 -.73027 0 0)" cx="-212.42" cy="-62.836" rx="36.007" ry="41.807"/>
								<ellipse transform="matrix(-.73717 -.67571 .68316 -.73027 0 0)" cx="-116.73" cy="-6.1428" rx="36.007" ry="41.807"/>
								<ellipse transform="matrix(-.73717 -.67571 .68316 -.73027 0 0)" cx="-230.51" cy="9.1266" rx="36.007" ry="41.807"/>
								<ellipse transform="matrix(-.73717 -.67571 .68316 -.73027 0 0)" cx="-199.39" cy="58.236" rx="36.007" ry="41.807"/>
								<ellipse transform="matrix(-.73717 -.67571 .68316 -.73027 0 0)" cx="-139.22" cy="47.193" rx="36.007" ry="41.807"/>
								<ellipse transform="matrix(-.73717 -.67571 .68316 -.73027 0 0)" cx="-175.45" cy="-4.1575" rx="36.007" ry="41.807"/>
							</g>
						</g>
					</svg>
					<h1>Login successful!</h1>
					<h2>You can close this window and return to the Jamsync CLI.</h2>
					<p>Your auth file is located at $HOME/.jamsyncauth</p>
				</main>
			</body>
		</html>`)

		log.Println("Successfully logged into Jamsync!")

		// close the HTTP server
		cleanup(server)
	})

	// parse the redirect URL for the port number
	u, err := url.Parse(redirectUrl)
	if err != nil {
		log.Printf("bad redirect URL: %s\n", err)
		os.Exit(1)
	}

	// set up a listener on the redirect port
	port := fmt.Sprintf(":%s", u.Port())
	l, err := net.Listen("tcp", port)
	if err != nil {
		log.Printf("can't listen to port %s: %s\n", port, err)
		os.Exit(1)
	}

	// open a browser window to the authorizationURL
	err = open.Start(authorizationURL)
	if err != nil {
		log.Printf("can't open browser to URL %s: %s\n", authorizationURL, err)
		os.Exit(1)
	}

	// start the blocking web server loop
	// this will exit when the handler gets fired and calls server.Close()
	server.Serve(l)
}

// getAccessToken trades the authorization code retrieved from the first OAuth2 leg for an access token
func getAccessToken(clientID string, codeVerifier string, authorizationCode string, callbackURL string) (string, error) {
	// set the url and form-encoded data for the POST to the access token endpoint
	url := fmt.Sprintf("https://%s/oauth/token", jamenv.Auth0Domain())
	data := fmt.Sprintf(
		"grant_type=authorization_code&client_id=%s"+
			"&code_verifier=%s"+
			"&code=%s"+
			"&redirect_uri=%s",
		clientID, codeVerifier, authorizationCode, callbackURL)
	payload := strings.NewReader(data)

	// create the request and execute it
	req, _ := http.NewRequest("POST", url, payload)
	req.Header.Add("content-type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("HTTP error: %s", err)
		return "", err
	}

	// process the response
	defer res.Body.Close()
	var responseData map[string]interface{}
	body, _ := ioutil.ReadAll(res.Body)

	// unmarshal the json into a string map
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		fmt.Printf("JSON error: %s", err)
		return "", err
	}

	// retrieve the access token out of the map, and return to caller
	accessToken := responseData["access_token"].(string)
	return accessToken, nil
}

// cleanup closes the HTTP server
func cleanup(server *http.Server) {
	// we run this as a goroutine so that this function falls through and
	// the socket to the browser gets flushed/closed before the server goes away
	go server.Close()
}

func InitConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	viper.SetConfigName(".jamsyncauth")
	viper.SetConfigType("json")
	viper.AddConfigPath(home)

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			configPath := home + "/.jamsyncauth"

			fmt.Printf("%s does not exist yet.\n", configPath)
			AuthorizeUser()
			err = viper.WriteConfigAs(configPath)
			if err != nil {
				return "", err
			}

			log.Println("Wrote config")
		} else {
			log.Println("$HOME/.jamsyncauth.json could not be read correctly. Try deleting this file and retrying.")
		}
	}
	return viper.Get("AccessToken").(string), nil
}

func ReauthConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	viper.SetConfigName(".jamsyncauth")
	viper.SetConfigType("json")
	viper.AddConfigPath(home)

	configPath := home + "/.jamsyncauth"

	AuthorizeUser()
	err = viper.WriteConfigAs(configPath)
	if err != nil {
		return "", err
	}

	log.Println("Wrote config")
	return viper.Get("AccessToken").(string), nil
}
