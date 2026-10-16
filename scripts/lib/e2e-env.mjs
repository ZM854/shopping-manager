import { randomBytes } from "node:crypto";
import { writeFileSync } from "node:fs";

export function ensureE2EEnv(path) {
  const content = [
    "# Только для изолированного локального E2E-окружения.",
    `E2E_JWT_ACCESS_SECRET=${randomBytes(32).toString("hex")}`,
    `E2E_JWT_REFRESH_SECRET=${randomBytes(32).toString("hex")}`,
    "",
  ].join("\n");
  try {
    writeFileSync(path, content, { flag: "wx", mode: 0o600 });
    return true;
  } catch (error) {
    if (error.code === "EEXIST") return false;
    throw error;
  }
}
