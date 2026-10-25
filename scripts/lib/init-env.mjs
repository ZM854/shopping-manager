import { randomBytes } from "node:crypto";
import {
  readFileSync,
  writeFileSync,
  existsSync,
  appendFileSync,
} from "node:fs";

// Only fill new media settings in an existing file; never rotate existing keys.
export function ensureLocalEnv(path, templatePath) {
  const template = readFileSync(templatePath, "utf8");
  if (!existsSync(path)) {
    const content = template
      .replace("GENERATE_ACCESS_SECRET", randomBytes(32).toString("hex"))
      .replace("GENERATE_REFRESH_SECRET", randomBytes(32).toString("hex"))
      .replace("GENERATE_S3_ACCESS_KEY", randomBytes(16).toString("hex"))
      .replace("GENERATE_S3_SECRET", randomBytes(32).toString("hex"));
    writeFileSync(path, content, { flag: "wx", mode: 0o600 });
    return "created";
  }
  const content = readFileSync(path, "utf8");
  const additions = template.split(/\r?\n/).filter((line) => {
    const key = line.match(/^(S3_[A-Z_]+)=/); // No unrelated settings or secrets.
    return (
      key &&
      !new RegExp(`^[\\t ]*(?:export[\\t ]+)?${key[1]}[\\t ]*=`, "m").test(
        content,
      )
    );
  });
  if (!additions.length) return "unchanged";
  const addition = additions
    .join("\n")
    .replace("GENERATE_S3_ACCESS_KEY", randomBytes(16).toString("hex"))
    .replace("GENERATE_S3_SECRET", randomBytes(32).toString("hex"));
  appendFileSync(path, `\n# Local recipe media storage\n${addition}\n`);
  return "updated";
}
