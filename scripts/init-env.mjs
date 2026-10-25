import { fileURLToPath } from "node:url";
import { ensureLocalEnv } from "./lib/init-env.mjs";

const path = fileURLToPath(new URL("../.env", import.meta.url));
const template = fileURLToPath(new URL("../.env.example", import.meta.url));
const result = ensureLocalEnv(path, template);
console.log(
  {
    created: "Создан .env с независимыми случайными JWT- и S3-ключами.",
    updated:
      "В .env добавлены отсутствующие S3-настройки; прежние значения сохранены.",
    unchanged: ".env уже содержит S3-настройки, файл сохранён без изменений.",
  }[result],
);
