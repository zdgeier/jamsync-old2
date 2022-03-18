import { Yap } from "./yap.ts";
import { gzip } from "https://deno.land/x/compress@v0.4.1/gzip/mod.ts";
import { Handlebars, HandlebarsConfig } from 'https://deno.land/x/handlebars/mod.ts';

const githubOAuthClientId = "57fe061763bd02f2aa4c";

const handle = new Handlebars({
    baseDir: 'views',
    extname: '.hbs',
    layoutsDir: 'layouts/',
    partialsDir: 'partials/',
    cachePartials: true,
    defaultLayout: 'main',
    helpers: undefined,
    compilerOptions: undefined,
});


async function handleIndexPage(
  requestEvent: Deno.RequestEvent,
) {
  try {
    const path = new URL(requestEvent.request.url)
    let code = path.searchParams.get("code")
    if (code) {
      const content: string = await handle.renderView('main', { clientId: githubOAuthClientId });
      const encodedIndexFile = gzip(new TextEncoder().encode(content));
      await requestEvent.respondWith(
        new Response(encodedIndexFile, {
          headers: [
            ["Content-Type", "text/html; charset=utf-8"],
            ["Content-Encoding", "gzip"],
            ["Strict-Transport-Security", "max-age=63072000"],
            ["X-Content-Type-Options", "nosniff"],
            ["X-Frame-Options", "SAMEORIGIN"],
          ],
          status: 200,
        }),
      );
    } else {
      const content: string = await handle.renderView('main', { clientId: githubOAuthClientId });
      const encodedIndexFile = gzip(new TextEncoder().encode(content));
      await requestEvent.respondWith(
        new Response(encodedIndexFile, {
          headers: [
            ["Content-Type", "text/html; charset=utf-8"],
            ["Content-Encoding", "gzip"],
            ["Strict-Transport-Security", "max-age=63072000"],
            ["X-Content-Type-Options", "nosniff"],
            ["X-Frame-Options", "SAMEORIGIN"],
          ],
          status: 200,
        }),
      );
    }
  } catch (error) {
    console.error(error);
  }
}

const images: Map<string, Uint8Array> = new Map();
const imagePath = (imageName: string) => `/images/${imageName}`;
for await (const dirEntry of Deno.readDir("images")) {
  if (dirEntry.isFile) {
    const path = imagePath(dirEntry.name);
    images.set(path, await Deno.readFile("." + path));
  }
}

async function handleFile(
  requestEvent: Deno.RequestEvent,
) {
  try {
    const path = (new URL(requestEvent.request.url)).pathname;

    await requestEvent.respondWith(
      new Response(images.get(path), {
        headers: [
          ["Content-Type", "image/webp"],
          ["Cache-Control", "max-age=31536000"],
        ],
        status: 200,
      }),
    );
  } catch (error) {
    console.error(error);
  }
}

const robotsFile = await Deno.readTextFile("./static/robots.txt");
async function handleRobotsPage(
  requestEvent: Deno.RequestEvent,
) {
  try {
    await requestEvent.respondWith(
      new Response(robotsFile, {
        headers: [["Content-Type", "text/plain"]],
        status: 200,
      }),
    );
  } catch (error) {
    console.error(error);
  }
}

const yap = new Yap();
yap.addPage("/", handleIndexPage, "2022-03-15");
yap.addPage("/index.html", handleIndexPage, "2022-03-15");
yap.addPage("/robots.txt", handleRobotsPage, "2022-03-15");
for (const dirPath of images.keys()) {
  yap.addPage(dirPath, handleFile, "2022-03-15");
}
await yap.start(8000);
