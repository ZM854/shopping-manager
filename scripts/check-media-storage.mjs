import { randomUUID } from "node:crypto";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const compose = [
  "compose",
  "--env-file",
  ".env.example",
  "-p",
  "shopping-manager-media-check",
  "-f",
  "compose.test.yaml",
  "-f",
  "compose.media-check.yaml",
];
function docker(args) {
  const result = spawnSync("docker", [...compose, ...args], {
    cwd: root,
    stdio: "inherit",
  });
  if (result.error || result.status !== 0)
    throw new Error(
      "Проверка медиа не выполнена: команда Docker завершилась ошибкой.",
    );
}
const id = randomUUID();
try {
  docker(["up", "-d", "--wait", "--wait-timeout", "120", "storage-test"]);
  const check = (phase, pattern) =>
    docker([
      "run",
      "--build",
      "--rm",
      "--no-deps",
      "-e",
      `TEST_MEDIA_PERSISTENCE_PHASE=${phase}`,
      "-e",
      `TEST_MEDIA_PERSISTENCE_ID=${id}`,
      "backend-test",
      "go",
      "test",
      "-count=1",
      "-tags=integration",
      "-run",
      pattern,
      "./integration",
    ]);
  check("write", "^TestMedia");
  docker([
    "up",
    "-d",
    "--force-recreate",
    "--wait",
    "--wait-timeout",
    "120",
    "storage-test",
  ]);
  check("read", "^TestMediaStorage$");
  console.log(
    "S3 CRUD, приватность, кодеки и сохранность после пересоздания проверены.",
  );
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
} finally {
  try {
    docker(["stop", "storage-test"]); // Keep the dedicated test volume; never delete user data.
  } catch {
    console.error("Не удалось остановить отдельный тестовый storage-test.");
    process.exitCode = 1;
  }
}
