import { getCookies } from "https://deno.land/std/http/cookie.ts";

// AWS has an official SDK that works with browsers. As most Deno Deploy's
// APIs are similar to browser's, the same SDK works with Deno Deploy.
// So we import the SDK along with some classes required to insert and
// retrieve data.
import { DynamoDBClient, createClient } from "https://denopkg.com/chiefbiiko/dynamodb/mod.ts";


// Create a client instance by providing your region information.
// The credentials are obtained from environment variables which
// we set during our project creation step on Deno Deploy.
const dynamoClient: DynamoDBClient = createClient({
  region: "us-east-1",
});

const server = Deno.listen({ port: 8000 });
console.log(`HTTP webserver running.  Access it at:  http://localhost:8000/`);

const githubOAuthClientId = "57fe061763bd02f2aa4c";
const indexHTML = Deno.readTextFile("./views/index.html");
const robotsHTML = Deno.readTextFile("./static/robots.txt");

for await (const conn of server) {
  serveHttp(conn);
}

async function serveHttp(conn: Deno.Conn) {
  const httpConn = Deno.serveHttp(conn);
  for await (const requestEvent of httpConn) {
    const url = new URL(requestEvent.request.url)
    const path = url.pathname;

    // placeholder
    let response = new Response("Invalid request", {
          headers: [
            ["Content-Type", "text/html; charset=utf-8"],
          ],
          status: 400,
        });

    if (path == "/" || path == "/index.html") {
      const indexHTMLString = await indexHTML;
      const code = url.searchParams.get("code");

      if (code) {
        const content: string = indexHTMLString.replace("{{clientId}}", githubOAuthClientId).replace("{{loggedIn}}", "true")
        // user has a code, do token handshake
        const postRequest = await fetch('https://github.com/login/oauth/access_token', {
          method: 'POST',
          headers: [
            ['Content-Type', 'application/json'],
            ['Accept', 'application/json']
          ],
          body: JSON.stringify({
            client_id: githubOAuthClientId,
            client_secret: "3110013ffd0530de1d55897dce926bd69c6cdac9",
            code: code,
          }),
        })

        // TODO: Encrypt access token to prevent CSS
        const accessTokenResponse = JSON.parse(await postRequest.text())
        response = new Response(content, {
            headers: [
              ["Content-Type", "text/html; charset=utf-8"],
              ["Set-Cookie", "jamsynctoken=" + accessTokenResponse.access_token + "; Max-Age=10000"]
            ],
            status: 200,
          });
      } else {
        const content: string = indexHTMLString.replace("{{clientId}}", githubOAuthClientId).replace("{{loggedIn}}", "false")
        // user has not logged in, respond with homepage
        response = new Response(content, {
            headers: [
              ["Content-Type", "text/html; charset=utf-8"],
            ],
            status: 200,
          })
      }
    } else if (path == "/api/githubuserinfo") {
      const cookies = getCookies(requestEvent.request.headers);

      if (cookies.jamsynctoken) {
        const emailResponse = await fetch("https://api.github.com/user", {
          headers: [
            ['Authorization', "token " + cookies.jamsynctoken],
          ]
        })

        const emailInfo = await emailResponse.json()

        response = new Response(JSON.stringify(emailInfo), {
            headers: [
              ["Content-Type", "text/html; charset=utf-8"],
            ],
            status: 200,
          })
      }
    } else if (path == "/version") {
      if (requestEvent.request.method == "GET") {
        try {
          // We grab the title form the request and send a GetItemCommand
          // to retrieve the information about the song.
          const searchParams = url.searchParams;
          const id = searchParams.get("id")!;
          let result = await dynamoClient.getItem({
            TableName: "jamsync_versions",
            Key: { id: 1 },
          });
      
          // The Item property contains all the data, so if it's not undefined,
          // we proceed to returning the information about the title
          if (result) {
            response = new Response(JSON.stringify({test: result.Item}), {
              headers: [
                ["Content-Type", "application/json; charset=utf-8"],
              ],
              status: 200,
            })
          }
        } catch (error) {
          console.log(error);
        } 
      }
    } else if (path == "/robots.txt") {
      const robotsHTMLString = await robotsHTML;
      response = new Response(robotsHTMLString, {
          headers: [
            ["Content-Type", "text/plain; charset=utf-8"],
          ],
          status: 200,
        })
    }

    await requestEvent.respondWith(response)
  }
}
