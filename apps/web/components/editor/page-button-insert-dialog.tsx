"use client";

import { useState } from "react";

import { toast } from "sonner";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  buttonActionSchema,
  type ButtonAction,
} from "@/lib/ast/schema";
import { newBlockId } from "@/lib/editor/ids";

type ActionKind = ButtonAction["type"];

export function PageButtonInsertDialog({
  open,
  onOpenChange,
  currentPageId,
  onInsert,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentPageId?: string;
  onInsert: (value: {
    label: string;
    confirmation?: string;
    actions: ButtonAction[];
  }) => void;
}) {
  const [label, setLabel] = useState("运行操作");
  const [confirmation, setConfirmation] = useState("确定执行这个页面按钮吗？");
  const [kind, setKind] = useState<ActionKind>("notification");
  const [targetPageId, setTargetPageId] = useState(currentPageId ?? "");
  const [insertText, setInsertText] = useState("");
  const [recordId, setRecordId] = useState("");
  const [entityId, setEntityId] = useState("");
  const [valuesJSON, setValuesJSON] = useState("{}");
  const [message, setMessage] = useState("");
  const [tone, setTone] = useState("success");
  const [webhookURL, setWebhookURL] = useState("");
  const [payloadJSON, setPayloadJSON] = useState("{}");

  const submit = () => {
    const trimmedLabel = label.trim();
    if (!trimmedLabel || trimmedLabel.length > 120) {
      toast.error("按钮名称需为 1-120 个字符");
      return;
    }
    let action: unknown;
    try {
      switch (kind) {
        case "insert_blocks":
          action = {
            type: kind,
            target_page_id: targetPageId.trim(),
            blocks: [
              {
                id: newBlockId(),
                type: "paragraph",
                content: [{ type: "text", text: insertText }],
              },
            ],
          };
          break;
        case "update_dataset_record":
          action = {
            type: kind,
            record_id: recordId.trim(),
            ...(entityId.trim() ? { entity_id: entityId.trim() } : {}),
            values: JSON.parse(valuesJSON),
          };
          break;
        case "notification":
          action = {
            type: kind,
            message: message.trim(),
            tone,
          };
          break;
        case "webhook":
          action = {
            type: kind,
            url: webhookURL.trim(),
            payload: JSON.parse(payloadJSON),
          };
          break;
      }
    } catch {
      toast.error("动作 JSON 不是合法对象");
      return;
    }
    const parsed = buttonActionSchema.safeParse(action);
    if (!parsed.success) {
      toast.error("按钮动作配置不完整", {
        description: parsed.error.issues[0]?.message,
      });
      return;
    }
    onInsert({
      label: trimmedLabel,
      ...(confirmation.trim() ? { confirmation: confirmation.trim() } : {}),
      actions: [parsed.data],
    });
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>插入页面按钮</DialogTitle>
          <DialogDescription>
            页面只在读者确认后执行；插入内容会进入 Proposal 审核。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <Label>
            按钮名称
            <Input value={label} onChange={(event) => setLabel(event.target.value)} />
          </Label>
          <Label>
            确认提示
            <Input
              value={confirmation}
              onChange={(event) => setConfirmation(event.target.value)}
            />
          </Label>
          <Label>
            动作
            <select
              value={kind}
              onChange={(event) => setKind(event.target.value as ActionKind)}
              className="mt-1 h-9 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
            >
              <option value="notification">显示通知</option>
              <option value="insert_blocks">插入内容块</option>
              <option value="update_dataset_record">修改数据记录</option>
              <option value="webhook">发送 Webhook</option>
            </select>
          </Label>
          {kind === "notification" ? (
            <>
              <Label>
                通知内容
                <Input
                  value={message}
                  onChange={(event) => setMessage(event.target.value)}
                />
              </Label>
              <Label>
                通知级别
                <select
                  value={tone}
                  onChange={(event) => setTone(event.target.value)}
                  className="mt-1 h-9 w-full rounded-lg border border-input bg-background px-2.5 text-sm"
                >
                  <option value="success">成功</option>
                  <option value="info">信息</option>
                  <option value="warning">警告</option>
                  <option value="error">错误</option>
                  <option value="default">默认</option>
                </select>
              </Label>
            </>
          ) : null}
          {kind === "insert_blocks" ? (
            <>
              <Label>
                目标页面 ID
                <Input
                  value={targetPageId}
                  onChange={(event) => setTargetPageId(event.target.value)}
                />
              </Label>
              <Label>
                插入段落
                <Textarea
                  value={insertText}
                  onChange={(event) => setInsertText(event.target.value)}
                />
              </Label>
            </>
          ) : null}
          {kind === "update_dataset_record" ? (
            <>
              <Label>
                记录 ID
                <Input
                  value={recordId}
                  onChange={(event) => setRecordId(event.target.value)}
                />
              </Label>
              <Label>
                Entity ID（可选）
                <Input
                  value={entityId}
                  onChange={(event) => setEntityId(event.target.value)}
                />
              </Label>
              <Label>
                Values JSON
                <Textarea
                  className="font-mono"
                  value={valuesJSON}
                  onChange={(event) => setValuesJSON(event.target.value)}
                />
              </Label>
            </>
          ) : null}
          {kind === "webhook" ? (
            <>
              <Label>
                公网 HTTPS URL
                <Input
                  type="url"
                  value={webhookURL}
                  onChange={(event) => setWebhookURL(event.target.value)}
                />
              </Label>
              <Label>
                Payload JSON
                <Textarea
                  className="font-mono"
                  value={payloadJSON}
                  onChange={(event) => setPayloadJSON(event.target.value)}
                />
              </Label>
            </>
          ) : null}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button type="button" onClick={submit}>
            插入按钮
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
