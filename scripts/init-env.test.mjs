import { strict as assert } from "node:assert";
import {
  mkdtempSync,
  readFileSync,
  writeFileSync,
  unlinkSync,
  rmdirSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { ensureLocalEnv } from "./lib/init-env.mjs";

function fixture(t) {
  const directory = mkdtempSync(join(tmpdir(), "shopping-init-"));
  const path = join(directory, ".env");
  const template = join(directory, "template");
  writeFileSync(
    template,
    "JWT_ACCESS_SECRET=GENERATE_ACCESS_SECRET\nJWT_REFRESH_SECRET=GENERATE_REFRESH_SECRET\nS3_PORT=8333\nS3_ACCESS_KEY_ID=GENERATE_S3_ACCESS_KEY\nS3_SECRET_ACCESS_KEY=GENERATE_S3_SECRET\n",
  );
  t.after(() => {
    unlinkSync(path);
    unlinkSync(template);
    rmdirSync(directory);
  });
  return { path, template };
}

test("init создаёт независимые ключи и не меняет их при повторе", (t) => {
  const { path, template } = fixture(t);
  assert.equal(ensureLocalEnv(path, template), "created");
  const content = readFileSync(path, "utf8");
  assert.equal(content.includes("GENERATE_"), false);
  const values = content.match(/=[a-f0-9]{32,64}/g);
  assert.equal(new Set(values).size, 4);
  assert.equal(ensureLocalEnv(path, template), "unchanged");
  assert.equal(readFileSync(path, "utf8"), content);
});

test("init дополняет старый env без изменения профиля и прежних секретов", (t) => {
  const { path, template } = fixture(t);
  const original =
    "# existing\nJWT_ACCESS_SECRET=keep-access\nJWT_REFRESH_SECRET=keep-refresh\nDB_NAME=keep-db\nS3_PORT=9000\n";
  writeFileSync(path, original);
  assert.equal(ensureLocalEnv(path, template), "updated");
  const content = readFileSync(path, "utf8");
  assert.equal(content.startsWith(original), true);
  assert.equal((content.match(/^S3_PORT=/gm) || []).length, 1);
  assert.equal(ensureLocalEnv(path, template), "unchanged");
  assert.equal(readFileSync(path, "utf8"), content);
});

test("init сохраняет ключи с допустимыми пробелами и export", (t) => {
  const { path, template } = fixture(t);
  const original =
    "  S3_PORT = 9000\n\texport S3_ACCESS_KEY_ID = existing-key\n  S3_SECRET_ACCESS_KEY = 'existing-secret'\n";
  writeFileSync(path, original);
  assert.equal(ensureLocalEnv(path, template), "unchanged");
  assert.equal(readFileSync(path, "utf8"), original);
});
