import { UCenterClient } from "../dist/index.js";

const baseURL = process.env.BASE_URL || "http://localhost:8080";
const client = new UCenterClient({ baseURL });
const login = await client.login({ tenant: "acme", email: "owner@acme.test", password: "Password123!" });
if (login.mfaRequired) throw new Error("unexpected MFA challenge");
const me = await client.me();
if (me.tenant.slug !== "acme") throw new Error("unexpected tenant");
const users = await client.listUsers({ pageSize: 5 });
if (!Array.isArray(users.items) || users.total < 1) throw new Error("users missing");
const overview = await client.getOverview();
if (typeof overview.stats.users !== "number") throw new Error("overview missing");
console.log(`✓ TypeScript SDK smoke passed (${users.total} users, ${overview.stats.clients} clients)`);
