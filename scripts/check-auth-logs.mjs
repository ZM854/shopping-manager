import { execFileSync } from "node:child_process";
import { randomBytes, randomUUID } from "node:crypto";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import { setTimeout } from "node:timers/promises";
import {
  assertPrivateLogs,
  assertPrivateURL,
  PrivacyCheckError,
} from "./lib/log-privacy.mjs";

// Только тестовый Compose; рабочие env, конфигурация и журналы не читаются.
const root = fileURLToPath(new URL("../", import.meta.url));
const composeArgs = [
  "compose",
  "--env-file",
  fileURLToPath(new URL("../.env.e2e.local", import.meta.url)),
  "--project-directory",
  root,
  "-f",
  fileURLToPath(new URL("../compose.e2e.yaml", import.meta.url)),
];
function compose(args) {
  try {
    return execFileSync("docker", [...composeArgs, ...args], {
      cwd: root,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
      maxBuffer: 32 * 1024 * 1024,
      timeout: 30_000,
    });
  } catch {
    // execFileSync.error содержит stdout/stderr: никогда не печатать объект ошибки.
    throw new PrivacyCheckError(
      "Не удалось проверить изолированный E2E Compose; вывод скрыт.",
    );
  }
}
function check(condition, message) {
  if (!condition) throw new PrivacyCheckError(message);
}
const origin = "http://127.0.0.1:55173";
const mailOrigin = "http://127.0.0.1:58025";
let phase = "nginx";
async function post(path, body, headers = {}) {
  return fetch(`${origin}/api${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Origin: origin, ...headers },
    body: JSON.stringify(body),
    signal: AbortSignal.timeout(10_000),
  });
}

async function main() {
  compose(["exec", "-T", "frontend", "nginx", "-t"]);
  phase = "HTTP и заголовки";
  const marker = `privacy-${randomUUID()}`;
  const activationProbe = randomUUID();
  const shareToken = randomBytes(32).toString("base64url");
  const password = `test-${randomUUID()}`;
  const secrets = [marker, activationProbe, shareToken, password];
  const since = new Date(Date.now() - 1000).toISOString();
  for (const path of [
    `/activation/${activationProbe}?probe=${marker}`,
    `/share#token=${shareToken}`,
  ]) {
    const response = await fetch(`${origin}${path}`, {
      signal: AbortSignal.timeout(10_000),
    });
    check(response.status === 200, "Страница проверки недоступна.");
    check(
      response.headers.get("referrer-policy") === "no-referrer",
      "Нет Referrer-Policy: no-referrer.",
    );
    check(
      /name="referrer"\s+content="no-referrer"/.test(await response.text()),
      "Нет no-referrer в HTML.",
    );
  }
  const probe = await fetch(`${origin}/api/healthz?probe=${marker}`, {
    headers: {
      Authorization: `Bearer ${marker}`,
      Cookie: `probe=${marker}`,
      Referer: `${origin}/activation/${activationProbe}?probe=${marker}`,
    },
    signal: AbortSignal.timeout(10_000),
  });
  check(probe.status === 200, "API проверки недоступно.");
  check(
    (await post("/activation", { token: activationProbe })).status === 400,
    "Неверный ответ проверки активации.",
  );
  const denied = await post(
    "/shopping-list-shares/import",
    { token: shareToken },
    { "Idempotency-Key": randomUUID() },
  );
  check(
    denied.status === 401 && denied.headers.get("cache-control") === "no-store",
    "Неверная защита гостевого импорта.",
  );
  const oversized = await post(`/recipes?probe=${marker}`, {
    steps: [{ text: `${marker}${"x".repeat(1048576)}` }],
  });
  check(
    oversized.status === 413 &&
      oversized.headers.get("cache-control") === "no-store" &&
      (await oversized.json()).code === "body_too_large",
    "Нет JSON 413 с no-store от собранного nginx.",
  );

  // Активированный тестовый аккаунт нужен для настоящего клиентского POST импорта.
  phase = "аккаунт и Mailpit";
  const email = `privacy-${randomUUID()}@example.test`;
  check(
    (await post("/registration", { name: "Проверка", email, password }))
      .status === 201,
    "Не удалось создать тестовый аккаунт.",
  );
  let activationToken;
  for (let i = 0; i < 40 && !activationToken; i++) {
    const inbox = await (
      await fetch(`${mailOrigin}/api/v1/messages`, {
        signal: AbortSignal.timeout(10_000),
      })
    ).json();
    const message = inbox.messages.find((message) =>
      message.To.some((to) => to.Address === email),
    );
    if (message) {
      const content = await (
        await fetch(`${mailOrigin}/api/v1/message/${message.ID}`, {
          signal: AbortSignal.timeout(10_000),
        })
      ).json();
      activationToken = `${content.Text}\n${content.HTML}`.match(
        /\/activation\/([A-Za-z0-9_-]+)/,
      )?.[1];
    }
    if (!activationToken) await setTimeout(100);
  }
  check(Boolean(activationToken), "Нет тестового письма активации.");
  secrets.push(activationToken);
  check(
    (await post("/activation", { token: activationToken })).status === 204,
    "Не удалось активировать тестовый аккаунт.",
  );
  const require = createRequire(
    new URL("../frontend/package.json", import.meta.url),
  );
  phase = "запуск браузера";
  const { chromium } = require("@playwright/test");
  const browser = await chromium.launch({
    channel:
      process.env.E2E_BROWSER_CHANNEL ??
      (process.platform === "win32" ? "msedge" : undefined),
  });
  let clientLogs = "";
  const incomingURLs = [];
  const referers = [];
  const authCaptures = [];
  async function captureAuth() {
    check(
      authCaptures.length > 0 &&
        (await Promise.all(authCaptures)).every(Boolean),
      "Не удалось прочитать тестовые auth-ответы; содержимое скрыто.",
    );
  }
  try {
    const page = await browser.newPage();
    page.on("console", (message) => {
      clientLogs += `${message.text()}\n`;
    });
    page.on("pageerror", (error) => {
      clientLogs += `${error.message}\n`;
    });
    page.on("response", (response) => {
      if (
        /\/api\/(login|refresh)$/.test(response.url()) &&
        response.status() === 200
      ) {
        // Читать до следующей навигации: браузер может удалить прежнее тело ответа.
        authCaptures.push(
          response.json().then(
            (body) => {
              if (!body || typeof body.accessToken !== "string") return false;
              secrets.push(body.accessToken);
              return true;
            },
            () => false,
          ),
        );
      }
    });
    page.on("request", (request) => {
      incomingURLs.push(request.url());
      referers.push(
        request.allHeaders().then(
          (headers) => ({
            present: Boolean(headers.referer),
            route: request.resourceType(),
          }),
          () => null,
        ),
      );
    });
    phase = "активация в браузере";
    await page.goto(`${origin}/activation/${activationProbe}`);
    await page.waitForURL(`${origin}/activation`);
    await page
      .getByRole("button", { name: "Подтвердить почту", exact: true })
      .click();
    await page.getByRole("alert").waitFor();
    phase = "вход в браузере";
    await page.goto(`${origin}/login`);
    await page.getByLabel("Email", { exact: true }).fill(email);
    await page.getByLabel("Пароль").fill(password);
    const login = page.waitForResponse((response) =>
      response.url().endsWith("/api/login"),
    );
    await page.getByRole("button", { name: "Войти", exact: true }).click();
    const response = await login;
    check(response.status() === 200, "Не удалось войти тестовым аккаунтом.");
    for (const cookie of await page.context().cookies())
      if (cookie.name === "refreshToken") secrets.push(cookie.value);
    await page.waitForURL(`${origin}/lists`);
    await captureAuth();
    phase = "импорт в браузере";
    await page.goto(`${origin}/share#token=${shareToken}`);
    const imported = page.waitForResponse((response) =>
      response.url().endsWith("/api/shopping-list-shares/import"),
    );
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Добавить", exact: true })
      .click();
    check(
      (await imported).status() === 404,
      "Неизвестный снимок должен возвращать 404.",
    );
    await page.getByRole("alert").waitFor();
    phase = "сохранение рецепта в браузере";
    await page.goto(`${origin}/recipes/new`);
    await page.getByLabel("Название рецепта", { exact: true }).fill(marker);
    await page.getByLabel("Категория блюда").selectOption("other");
    await page.getByLabel("Название ингредиента 1").fill(marker);
    await page.getByLabel("Описание шага 1").fill(marker);
    const created = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/recipes") &&
        response.request().method() === "POST",
    );
    await page
      .getByRole("button", { name: "Сохранить рецепт", exact: true })
      .click();
    const recipe = await created;
    check(
      recipe.status() === 201 &&
        recipe.headers()["cache-control"] === "no-store",
      "Не удалось сохранить тестовый рецепт.",
    );
    await page.waitForURL(/\/recipes\/[1-9][0-9]*$/);
    phase = "проверка клиентских данных";
    await captureAuth();
    for (const cookie of await page.context().cookies())
      if (cookie.name === "refreshToken") secrets.push(cookie.value);
    assertPrivateLogs(clientLogs, secrets);
    for (const url of incomingURLs) assertPrivateURL(url, [shareToken]);
    const headers = await Promise.all(referers);
    check(
      headers.every((header) => header !== null),
      "Не удалось прочитать заголовки браузера; содержимое скрыто.",
    );
    const leakedReferers = headers.filter((referer) => referer.present);
    check(
      leakedReferers.length === 0,
      `Нет no-referrer у браузерного запроса (${leakedReferers.map((referer) => referer.route).join(", ")}).`,
    );
    const storage = await page.evaluate(() =>
      JSON.stringify({
        local: { ...localStorage },
        session: { ...sessionStorage },
      }),
    );
    check(
      !secrets.some((secret) => storage.includes(secret)),
      "Нет защиты секретов в browser storage.",
    );
    check(
      incomingURLs.some((url) =>
        url.endsWith("/api/shopping-list-shares/import"),
      ),
      "Нет настоящего клиентского запроса импорта.",
    );
  } finally {
    await browser.close();
  }

  // Docker читает оба потока, включая stderr nginx. Ни один фрагмент не выводится.
  phase = "проверка журналов контейнеров";
  let logs = "";
  for (let i = 0; i < 20; i++) {
    logs = compose([
      "logs",
      "--no-color",
      "--since",
      since,
      "frontend",
      "backend",
    ]).replace(/\u001b\[[0-9;]*m/g, "");
    assertPrivateLogs(logs, secrets);
    if (
      logs.includes("GET /api/healthz HTTP/") &&
      logs.includes("path=/api/shopping-list-shares/import") &&
      logs.includes("path=/api/recipes")
    )
      break;
    await setTimeout(100);
  }
  check(
    logs.includes("GET /api/healthz HTTP/"),
    "Не найдены обычные записи nginx; проверка неполна.",
  );
  check(
    logs.includes("path=/api/shopping-list-shares/import"),
    "Не найдены обычные записи API; проверка неполна.",
  );
  check(
    logs.includes("path=/api/recipes"),
    "Не найдены обычные записи API рецептов; проверка неполна.",
  );
  console.log(
    "Проверены nginx, JSON 413, no-referrer, URL и журналы nginx/API/клиента: секреты и содержимое рецепта отсутствуют.",
  );
}

try {
  await main();
} catch (error) {
  // Только собственные безопасные сообщения; ошибки библиотек могут содержать URL.
  console.error(
    error instanceof PrivacyCheckError
      ? error.message
      : `Проверка приватности E2E не завершилась (этап: ${phase}); диагностический вывод скрыт.`,
  );
  process.exitCode = 1;
}
