// AST v1 稳定序列化（与 backend/internal/ast/serialize.go 逐字节一致）。
//
// Canonical 规则：
//   - 对象键排序：v1 键全为 ASCII，按码元序（JS 字符串比较）与
//     Go 的 UTF-8 字节序结果相同；
//   - 无任何空白；
//   - 字符串：仅转义 '"'、'\\' 与 < 0x20 的控制字符
//     （\b \f \n \r \t 用短转义，其余用 \u00xx 小写 hex），
//     其余字符（含非 ASCII 与 '<' '>' '&'）保留原始 UTF-8，不做 HTML 转义；
//   - 数字：有限 IEEE 754 双精度值按最短十进制形式输出，禁止指数记法；
//     超出安全整数范围（|n| > 2^53-1）抛错。
//
// 输入为 JSON.parse / Zod parse 产出的普通对象；undefined 值按 JSON 语义忽略。

const MAX_SAFE_INTEGER = Number.MAX_SAFE_INTEGER; // 2^53 - 1

function expandExponential(value: string): string {
  const [coefficient, exponentText] = value.toLowerCase().split("e");
  if (exponentText === undefined) return value;
  const negative = coefficient.startsWith("-");
  const unsigned = negative ? coefficient.slice(1) : coefficient;
  const [integer, fraction = ""] = unsigned.split(".");
  const digits = integer + fraction;
  const decimalIndex = integer.length + Number(exponentText);
  let expanded: string;
  if (decimalIndex <= 0) {
    expanded = "0." + "0".repeat(-decimalIndex) + digits;
  } else if (decimalIndex >= digits.length) {
    expanded = digits + "0".repeat(decimalIndex - digits.length);
  } else {
    expanded =
      digits.slice(0, decimalIndex) + "." + digits.slice(decimalIndex);
  }
  return negative ? "-" + expanded : expanded;
}

function escapeString(s: string): string {
  let out = '"';
  // for..of 按码点迭代；代理对按原样输出（与 Go 的 rune 迭代一致）。
  for (const ch of s) {
    const cp = ch.codePointAt(0)!;
    switch (ch) {
      case '"':
        out += '\\"';
        break;
      case "\\":
        out += "\\\\";
        break;
      case "\b":
        out += "\\b";
        break;
      case "\f":
        out += "\\f";
        break;
      case "\n":
        out += "\\n";
        break;
      case "\r":
        out += "\\r";
        break;
      case "\t":
        out += "\\t";
        break;
      default:
        if (cp < 0x20) {
          out += "\\u" + cp.toString(16).padStart(4, "0");
        } else {
          out += ch;
        }
    }
  }
  return out + '"';
}

function writeCanonical(value: unknown): string {
  if (value === null) return "null";
  if (typeof value === "boolean") return value ? "true" : "false";
  if (typeof value === "string") return escapeString(value);
  if (typeof value === "number") {
    if (!Number.isFinite(value) || Math.abs(value) > MAX_SAFE_INTEGER) {
      throw new Error(`canonical 数字超出有限安全范围，得到 ${value}`);
    }
    if (Object.is(value, -0)) return "0";
    return expandExponential(String(value));
  }
  if (Array.isArray(value)) {
    return "[" + value.map(writeCanonical).join(",") + "]";
  }
  if (typeof value === "object") {
    const keys = Object.keys(value as Record<string, unknown>).filter(
      (k) => (value as Record<string, unknown>)[k] !== undefined,
    );
    // JS 字符串比较按 UTF-16 码元序；v1 键全为 ASCII，与 Go 字节序一致。
    keys.sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));
    const parts = keys.map(
      (k) =>
        escapeString(k) +
        ":" +
        writeCanonical((value as Record<string, unknown>)[k]),
    );
    return "{" + parts.join(",") + "}";
  }
  throw new Error(`canonical 不支持的类型: ${typeof value}`);
}

/** canonicalJson 输出 value 的确定性字符串表示，供哈希与快照存储。 */
export function canonicalJson(value: unknown): string {
  return writeCanonical(value);
}
