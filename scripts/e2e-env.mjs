import { fileURLToPath } from "node:url";
import { ensureE2EEnv } from "./lib/e2e-env.mjs";

const path = fileURLToPath(new URL("../.env.e2e.local", import.meta.url));
console.log(
  ensureE2EEnv(path)
    ? "Создан .env.e2e.local с независимыми тестовыми JWT-секретами."
    : ".env.e2e.local уже существует, сохранён без изменений.",
);
