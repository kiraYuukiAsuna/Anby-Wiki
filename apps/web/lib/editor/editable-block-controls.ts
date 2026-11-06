import type { ViewMutationRecord } from "@tiptap/pm/view";

export interface EditableBlockEditor {
  readonly isEditable?: boolean;
  updateBlock?(
    block: string | { id: string },
    update: { props?: Record<string, unknown>; children?: unknown[] },
  ): unknown;
  readonly document?: unknown[];
}

const CONTROL_EVENTS = [
  "beforeinput",
  "input",
  "keydown",
  "keyup",
  "keypress",
  "mousedown",
  "mouseup",
  "click",
  "dblclick",
  "paste",
  "drop",
] as const;

export function patchBlockProps(
  editor: EditableBlockEditor,
  blockId: string,
  props: Record<string, unknown>,
): void {
  if (editor.isEditable === false || !editor.updateBlock) return;
  editor.updateBlock(blockId, { props });
}

export function stringProp(
  props: Record<string, unknown>,
  key: string,
  fallback = "",
): string {
  const value = props[key];
  return typeof value === "string" ? value : fallback;
}

export function numberProp(
  props: Record<string, unknown>,
  key: string,
  fallback = 0,
): number {
  const value = props[key];
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

export function booleanProp(
  props: Record<string, unknown>,
  key: string,
  fallback = false,
): boolean {
  const value = props[key];
  return typeof value === "boolean" ? value : fallback;
}

export function formatJSONText(value: string, fallback: unknown): string {
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return JSON.stringify(fallback, null, 2);
  }
}

function bindEditorControl(element: HTMLElement): void {
  element.dataset.editorControl = "true";
  element.contentEditable = "false";
  for (const type of CONTROL_EVENTS) {
    element.addEventListener(type, (event) => event.stopPropagation());
  }
}

export function ignoreEditableBlockMutation(root: HTMLElement) {
  return (mutation: ViewMutationRecord): boolean => {
    if (mutation.type === "selection") {
      const active = root.ownerDocument.activeElement;
      return active !== null && root.contains(active);
    }
    return root.contains(mutation.target);
  };
}

export function createEditableBlockShell(options: {
  type: string;
  title: string;
  summary?: string;
  className?: string;
}): HTMLElement {
  const root = document.createElement("details");
  root.className = ["ast-editable-block", options.className]
    .filter(Boolean)
    .join(" ");
  root.dataset.editorBlockType = options.type;
  root.contentEditable = "false";

  const summaryWrapper = document.createElement("summary");
  summaryWrapper.className = "ast-editable-block__summary-trigger";

  const header = document.createElement("div");
  header.className = "ast-editable-block__header";

  const title = document.createElement("span");
  title.className = "ast-editable-block__title";
  title.textContent = options.title;
  header.appendChild(title);

  if (options.summary) {
    const summary = document.createElement("span");
    summary.className = "ast-editable-block__summary";
    summary.textContent = options.summary;
    header.appendChild(summary);
  }

  const hint = document.createElement("span");
  hint.className = "ast-editable-block__hint";
  hint.textContent = "点击编辑";
  header.appendChild(hint);

  summaryWrapper.appendChild(header);
  root.appendChild(summaryWrapper);
  return root;
}

export function createFieldGrid(...fields: HTMLElement[]): HTMLElement {
  const grid = document.createElement("div");
  grid.className = "ast-editable-block__grid";
  for (const field of fields) grid.appendChild(field);
  return grid;
}

export function createPreview(text: string): HTMLElement {
  const preview = document.createElement("div");
  preview.className = "ast-editable-block__preview";
  preview.textContent = text;
  return preview;
}

function fieldShell(labelText: string): {
  wrapper: HTMLLabelElement;
  error: HTMLSpanElement;
} {
  const wrapper = document.createElement("label");
  wrapper.className = "ast-editable-block__field";

  const label = document.createElement("span");
  label.className = "ast-editable-block__label";
  label.textContent = labelText;
  wrapper.appendChild(label);

  const error = document.createElement("span");
  error.className = "ast-editable-block__error";
  error.setAttribute("role", "alert");

  return { wrapper, error };
}

function setFieldError(
  control: HTMLElement,
  error: HTMLElement,
  message: string | null,
): void {
  if (message) {
    control.setAttribute("aria-invalid", "true");
    error.textContent = message;
    if (!error.parentElement) control.after(error);
  } else {
    control.removeAttribute("aria-invalid");
    error.textContent = "";
    error.remove();
  }
}

export function textField(options: {
  label: string;
  value: string;
  onCommit: (value: string) => void;
  disabled?: boolean;
  placeholder?: string;
  multiline?: boolean;
  rows?: number;
  monospace?: boolean;
  validate?: (value: string) => string | null;
}): HTMLElement {
  const { wrapper, error } = fieldShell(options.label);
  const control = options.multiline
    ? document.createElement("textarea")
    : document.createElement("input");
  if (!options.multiline) {
    (control as HTMLInputElement).type = "text";
  } else {
    (control as HTMLTextAreaElement).rows = options.rows ?? 3;
  }
  control.className = [
    "ast-editable-block__control",
    options.multiline ? "ast-editable-block__textarea" : "",
    options.monospace ? "ast-editable-block__control--mono" : "",
  ]
    .filter(Boolean)
    .join(" ");
  control.value = options.value;
  control.placeholder = options.placeholder ?? "";
  control.disabled = options.disabled ?? false;
  bindEditorControl(control);

  const commit = () => {
    const message = options.validate?.(control.value) ?? null;
    if (message) {
      setFieldError(control, error, message);
      return;
    }
    setFieldError(control, error, null);
    if (control.value !== options.value) options.onCommit(control.value);
  };

  control.addEventListener("change", commit);
  control.addEventListener("input", () => setFieldError(control, error, null));
  control.addEventListener("keydown", (event) => {
    const keyEvent = event as KeyboardEvent;
    if (
      keyEvent.key === "Enter" &&
      (!options.multiline || keyEvent.metaKey || keyEvent.ctrlKey)
    ) {
      keyEvent.preventDefault();
      commit();
      control.blur();
      return;
    }
    if (keyEvent.key === "Escape") {
      keyEvent.preventDefault();
      control.value = options.value;
      setFieldError(control, error, null);
      control.blur();
    }
  });

  wrapper.appendChild(control);
  return wrapper;
}

export function numberField(options: {
  label: string;
  value: number;
  onCommit: (value: number) => void;
  disabled?: boolean;
  min?: number;
  max?: number;
  step?: number;
  integer?: boolean;
  emptyValue?: number;
}): HTMLElement {
  const { wrapper, error } = fieldShell(options.label);
  const control = document.createElement("input");
  control.type = "number";
  control.className = "ast-editable-block__control";
  control.value = Number.isFinite(options.value) ? String(options.value) : "";
  if (options.min !== undefined) control.min = String(options.min);
  if (options.max !== undefined) control.max = String(options.max);
  if (options.step !== undefined) control.step = String(options.step);
  control.disabled = options.disabled ?? false;
  bindEditorControl(control);

  const commit = () => {
    const raw = control.value.trim();
    const next = raw === "" && options.emptyValue !== undefined
      ? options.emptyValue
      : Number(raw);
    if (!Number.isFinite(next)) {
      setFieldError(control, error, "请输入数字");
      return;
    }
    if (options.integer && !Number.isInteger(next)) {
      setFieldError(control, error, "请输入整数");
      return;
    }
    if (options.min !== undefined && next < options.min) {
      setFieldError(control, error, `不能小于 ${options.min}`);
      return;
    }
    if (options.max !== undefined && next > options.max) {
      setFieldError(control, error, `不能大于 ${options.max}`);
      return;
    }
    setFieldError(control, error, null);
    if (next !== options.value) options.onCommit(next);
  };

  control.addEventListener("change", commit);
  control.addEventListener("input", () => setFieldError(control, error, null));
  control.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      commit();
      control.blur();
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      control.value = String(options.value);
      setFieldError(control, error, null);
      control.blur();
    }
  });

  wrapper.appendChild(control);
  return wrapper;
}

export function selectField(options: {
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onCommit: (value: string) => void;
  disabled?: boolean;
}): HTMLElement {
  const { wrapper } = fieldShell(options.label);
  const control = document.createElement("select");
  control.className = "ast-editable-block__control";
  control.value = options.value;
  control.disabled = options.disabled ?? false;
  bindEditorControl(control);
  for (const option of options.options) {
    const item = document.createElement("option");
    item.value = option.value;
    item.textContent = option.label;
    control.appendChild(item);
  }
  control.value = options.value;
  control.addEventListener("change", () => options.onCommit(control.value));
  wrapper.appendChild(control);
  return wrapper;
}

export function checkboxField(options: {
  label: string;
  checked: boolean;
  onCommit: (checked: boolean) => void;
  disabled?: boolean;
}): HTMLElement {
  const wrapper = document.createElement("label");
  wrapper.className = "ast-editable-block__checkbox-row";
  const control = document.createElement("input");
  control.type = "checkbox";
  control.checked = options.checked;
  control.disabled = options.disabled ?? false;
  bindEditorControl(control);
  control.addEventListener("change", () => options.onCommit(control.checked));
  const label = document.createElement("span");
  label.textContent = options.label;
  wrapper.append(control, label);
  return wrapper;
}

export function buttonControl(options: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  variant?: "default" | "danger";
}): HTMLButtonElement {
  const control = document.createElement("button");
  control.type = "button";
  control.className = [
    "ast-editable-block__button",
    options.variant === "danger" ? "ast-editable-block__button--danger" : "",
  ]
    .filter(Boolean)
    .join(" ");
  control.textContent = options.label;
  control.disabled = options.disabled ?? false;
  bindEditorControl(control);
  control.addEventListener("click", (event) => {
    event.preventDefault();
    options.onClick();
  });
  return control;
}

export function createActionRow(...actions: HTMLElement[]): HTMLElement {
  const row = document.createElement("div");
  row.className = "ast-editable-block__actions";
  row.append(...actions);
  return row;
}

export function jsonTextareaField(options: {
  label: string;
  value: string;
  fallback: unknown;
  onCommit: (jsonText: string, parsed: unknown) => void;
  disabled?: boolean;
  rows?: number;
  validate?: (parsed: unknown) => string | null;
}): HTMLElement {
  return textField({
    label: options.label,
    value: formatJSONText(options.value, options.fallback),
    disabled: options.disabled,
    multiline: true,
    rows: options.rows ?? 4,
    monospace: true,
    validate: (value) => {
      try {
        const parsed = JSON.parse(value) as unknown;
        return options.validate?.(parsed) ?? null;
      } catch {
        return "请输入合法 JSON";
      }
    },
    onCommit: (value) => {
      const parsed = JSON.parse(value) as unknown;
      options.onCommit(JSON.stringify(parsed), parsed);
    },
  });
}

export function nonEmpty(message = "不能为空") {
  return (value: string): string | null =>
    value.trim().length > 0 ? null : message;
}

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function requiredUUID(value: string): string | null {
  return UUID_RE.test(value.trim()) ? null : "请输入合法 UUID";
}

export function optionalUUID(value: string): string | null {
  const trimmed = value.trim();
  return trimmed === "" || UUID_RE.test(trimmed) ? null : "请输入合法 UUID";
}

export function requiredHTTPURL(value: string): string | null {
  try {
    const parsed = new URL(value);
    return parsed.protocol === "http:" || parsed.protocol === "https:"
      ? null
      : "请输入 HTTP(S) URL";
  } catch {
    return "请输入 HTTP(S) URL";
  }
}

export function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}
