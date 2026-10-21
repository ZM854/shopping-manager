import { strict as assert } from "node:assert";
import { test } from "node:test";
import { assertPrivateLogs, assertPrivateURL } from "./lib/log-privacy.mjs";

test("проверка обнаруживает каждый секрет, не раскрывая его в ошибке", () => {
  const secrets = [
    "test-password-private",
    "test-activation-private",
    "test-share-private",
  ];
  for (const secret of secrets) {
    assert.throws(
      () => assertPrivateLogs(`prefix ${secret} suffix`, secrets),
      (error) => {
        assert.equal(error.message.includes(secret), false);
        assert.match(error.message, /обнаружен тестовый секрет/);
        return true;
      },
    );
  }
  assert.doesNotThrow(() =>
    assertPrivateLogs("POST /api/shopping-list-shares/import 404", secrets),
  );
});

test("проверка URL обнаруживает секрет в path, query и fragment", () => {
  const secret = "test-share-private";
  for (const url of [
    `/api/${secret}`,
    `/share?token=${secret}`,
    `/share#token=${secret}`,
  ]) {
    assert.throws(() => assertPrivateURL(url, [secret]), /адрес скрыт/);
  }
  assert.doesNotThrow(() =>
    assertPrivateURL("/api/shopping-list-shares/import", [secret]),
  );
});
