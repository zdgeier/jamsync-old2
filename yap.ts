export type YapRouteHandler = (
  requestEvent: Deno.RequestEvent,
) => void;

export class Yap {
  routes: Map<string, YapRouteHandler>;
  lastmod: Map<string, string>;
  publishingRoutes: Map<string, YapRouteHandler>;

  constructor() {
    this.routes = new Map();
    this.lastmod = new Map();
    this.publishingRoutes = new Map();
  }

  addPublishingRoute(url: string, handler: YapRouteHandler) {
    this.publishingRoutes.set(url, handler);
  }

  addPage(url: string, handler: YapRouteHandler, lastmod: string) {
    this.routes.set(url, handler);
    this.lastmod.set(url, lastmod);
  }

  addPageSet(url: string, handler: YapRouteHandler) {
    this.routes.set(`${url}.html`, handler);
  }

  async start(port: number) {
    this.addPublishingRoute("/sitemap.xml", this.handleSitemapPage.bind(this));
    this.addPublishingRoute(
      "/sitemap.json",
      this.handleSitemapJsonPage.bind(this),
    );

    const server = Deno.listen({ port: port });
    console.log(
      `HTTP webserver running.  Access it at:  http://localhost:8000/`,
    );

    for await (const conn of server) {
      this.serveHttp(conn);
    }
  }

  async serveHttp(conn: Deno.Conn) {
    const httpConn = Deno.serveHttp(conn);
    for await (const requestEvent of httpConn) {
      const path = (new URL(requestEvent.request.url)).pathname;
      const handler = this.routes.get(path);
      const publishHandler = this.publishingRoutes.get(path);

      if (handler) {
        handler(requestEvent);
      } else if (publishHandler) {
        publishHandler(requestEvent);
      } else {
        requestEvent.respondWith(
          new Response("404 not found", {
            status: 404,
          }),
        );
      }
    }
  }

  handleSitemapJsonPage(requestEvent: Deno.RequestEvent) {
    try {
      const urls: string[] = Array.from(this.routes.keys());

      requestEvent.respondWith(
        new Response(JSON.stringify(urls), {
          status: 200,
        }),
      );
    } catch (error) {
      console.error(error);
    }
  }

  handleSitemapPage(requestEvent: Deno.RequestEvent) {
    try {
      let content = "";
      for (const route of this.routes.keys()) {
        content = content + `<url>
                        <loc>https://jamsync.io${route}</loc>
                        <lastmod>${this.lastmod.get(route)}</lastmod>
                       </url>`;
      }

      const body = `<?xml version="1.0" encoding="UTF-8"?>
                    <urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
                      ${content}
                    </urlset>`;

      requestEvent.respondWith(
        new Response(body, {
          headers: [["Content-Type", "text/xml"]],
          status: 200,
        }),
      );
    } catch (error) {
      console.error(error);
    }
  }
}
