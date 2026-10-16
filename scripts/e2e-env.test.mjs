import { strict as assert } from "node:assert";
import { mkdtempSync, readFileSync, rmdirSync, unlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { ensureE2EEnv } from "./lib/e2e-env.mjs";

test("E2E env создаёт разные секреты и не перезаписывает существующий файл", (t) => {
  const directory = mkdtempSync(join(tmpdir(), "shopping-e2e-env-"));
  const file = join(directory, ".env.e2e.local");
  t.after(() => {
    unlinkSync(file);
    rmdirSync(directory);
  });
  assert.equal(ensureE2EEnv(file), true);
  const before = readFileSync(file, "utf8");
  const values = before.match(/=[a-f0-9]{64}/g);
  assert.equal(values.length, 2);
  assert.notEqual(values[0], values[1]);
  assert.equal(ensureE2EEnv(file), false);
  assert.equal(readFileSync(file, "utf8"), before);
});

test("E2E env не скрывает ошибки записи, отличные от существующего файла", (t) => {
  const directory = mkdtempSync(join(tmpdir(), "shopping-e2e-env-"));
  t.after(() => rmdirSync(directory));
  assert.throws(
    () => ensureE2EEnv(join(directory, "missing", ".env.e2e.local")),
    {
      code: "ENOENT",
    },
  );
});
