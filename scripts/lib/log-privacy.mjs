// Ошибки намеренно не содержат проверяемый текст, найденный секрет или строку лога.
export class PrivacyCheckError extends Error {}

export function assertPrivateLogs(logs, secrets) {
  if (secrets.some((secret) => secret && logs.includes(secret))) {
    throw new PrivacyCheckError(
      "В журналах обнаружен тестовый секрет; содержимое скрыто.",
    );
  }
}

export function assertPrivateURL(url, secrets) {
  if (secrets.some((secret) => secret && url.includes(secret))) {
    throw new PrivacyCheckError(
      "Секрет обмена попал в URL HTTP-запроса; адрес скрыт.",
    );
  }
}
