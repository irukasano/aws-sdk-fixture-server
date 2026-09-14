function endpointSnapshot() {
  return Object.fromEntries(Object.entries(process.env).filter(([key]) => key === "AWS_ENDPOINT_URL" || key.startsWith("AWS_ENDPOINT_URL_")));
}

function restoreEndpoints(snapshot) {
  for (const key of Object.keys(process.env)) {
    if (key === "AWS_ENDPOINT_URL" || key.startsWith("AWS_ENDPOINT_URL_")) delete process.env[key];
  }
  Object.assign(process.env, snapshot);
}

async function request(url, method, payload) {
  const response = await fetch(url, {
    method,
    headers: payload === undefined ? undefined : { "content-type": "application/json" },
    body: payload === undefined ? undefined : JSON.stringify(payload),
  });
  const text = await response.text();
  if (!response.ok) throw new Error(`${method} ${url} failed: ${response.status} ${text}`);
  return text ? JSON.parse(text) : undefined;
}

export class FixtureSession {
  static async start({ serverUrl }) {
    const created = await request(`${serverUrl}/__fixture/sessions`, "POST");
    if (!created?.sessionId || !created?.endpoint || !created?.issuer) throw new Error("invalid session response");
    const savedEndpoints = endpointSnapshot();
    for (const key of Object.keys(savedEndpoints)) delete process.env[key];
    process.env.AWS_ENDPOINT_URL = created.endpoint;
    return new FixtureSession(serverUrl, created.sessionId, created.issuer, savedEndpoints);
  }

  constructor(serverUrl, sessionId, issuer, savedEndpoints) { this.serverUrl = serverUrl; this.sessionId = sessionId; this.issuer = issuer; this.savedEndpoints = savedEndpoints; }
  control(path) { return `${this.serverUrl}/__fixture/sessions/${this.sessionId}${path}`; }
  async loadScenario(path) { const loaded = await request(this.control("/scenario"), "POST", { path }); if (loaded?.issuer) this.issuer = loaded.issuer; return loaded; }
  reset() { return request(this.control("/reset"), "POST"); }
  requests() { return request(this.control("/requests"), "GET"); }
  async destroy() { try { await request(this.control(""), "DELETE"); } finally { restoreEndpoints(this.savedEndpoints); } }
}
