const currentEnv = import.meta.env.VITE_ENV || "dev";
const isDev = currentEnv === "dev";

const styles = {
  info: "color: #2196F3; font-weight: bold;",
  warn: "color: #FF9800; font-weight: bold;",
  error: "color: #F44336; font-weight: bold;",
  debug: "font-weight: bold;",
};

export const logger = {
  info(tag: string, message: string, data?: unknown) {
    if (!isDev) return;
    console.log(
      `%c[INFO][${tag}] %c${message}`,
      styles.info,
      "color: inherit",
      data ?? "",
    );
  },

  debug(tag: string, message: string, data?: unknown) {
    if (!isDev) return;
    console.log(
      `%c[OK][${tag}] %c${message}`,
      styles.debug,
      "color: inherit",
      data ?? "",
    );
  },

  warn(tag: string, message: string, data?: unknown) {
    if (!isDev) return;
    console.warn(`%c[WARN][${tag}] ${message}`, styles.warn, data ?? "");
  },

  error(tag: string, message: string, error?: unknown) {
    console.error(`%c[ERROR][${tag}] ${message}`, styles.error, error ?? "");
  },
};
