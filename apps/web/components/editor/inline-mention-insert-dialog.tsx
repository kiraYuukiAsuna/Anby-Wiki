"use client";

import { useState } from "react";

import { CalendarDays, Database, UserRound } from "lucide-react";
import { toast } from "sonner";
import useSWR from "swr";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  databaseMentionNodeSchema,
  dateMentionNodeSchema,
  mentionNodeSchema,
} from "@/lib/ast/schema";
import { datasetsApi } from "@/lib/api";

type MentionMode = "person" | "date" | "database";

export function InlineMentionInsertDialog({
  open,
  onOpenChange,
  onInsertPerson,
  onInsertDate,
  onInsertDatabase,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onInsertPerson: (actorId: string, displayText: string) => void;
  onInsertDate: (date: string, endDate: string, displayText: string) => void;
  onInsertDatabase: (datasetId: string, displayText: string) => void;
}) {
  const [mode, setMode] = useState<MentionMode>("person");
  const [actorId, setActorId] = useState("");
  const [personText, setPersonText] = useState("");
  const [date, setDate] = useState("");
  const [endDate, setEndDate] = useState("");
  const [dateText, setDateText] = useState("");
  const [datasetId, setDatasetId] = useState("");
  const [databaseText, setDatabaseText] = useState("");

  const { data: datasetPage, isLoading: datasetsLoading } = useSWR(
    open && mode === "database" ? ["editor-dataset-mentions"] : null,
    () => datasetsApi().listDatasets({ pageSize: 100 }),
  );
  const datasets = datasetPage?.items ?? [];

  const close = () => onOpenChange(false);

  const submit = () => {
    if (mode === "person") {
      const parsed = mentionNodeSchema.safeParse({
        type: "mention",
        actor_id: actorId.trim(),
        display_text: personText.trim(),
      });
      if (!parsed.success || !parsed.data.display_text) {
        toast.error("Person 提及需要有效 Actor ID 和显示名称");
        return;
      }
      onInsertPerson(parsed.data.actor_id, parsed.data.display_text);
      close();
      return;
    }

    if (mode === "date") {
      const displayText =
        dateText.trim() || (endDate ? `${date} 至 ${endDate}` : date);
      const parsed = dateMentionNodeSchema.safeParse({
        type: "date_mention",
        date,
        ...(endDate ? { end_date: endDate } : {}),
        display_text: displayText,
      });
      if (
        !parsed.success ||
        !parsed.data.display_text ||
        (parsed.data.end_date && parsed.data.end_date < parsed.data.date)
      ) {
        toast.error("请选择有效日期，结束日期不能早于开始日期");
        return;
      }
      onInsertDate(
        parsed.data.date,
        parsed.data.end_date ?? "",
        parsed.data.display_text,
      );
      close();
      return;
    }

    const selected = datasets.find((dataset) => dataset.id === datasetId);
    const parsed = databaseMentionNodeSchema.safeParse({
      type: "database_mention",
      dataset_id: datasetId,
      display_text: databaseText.trim() || selected?.name || "",
    });
    if (!parsed.success || !parsed.data.display_text) {
      toast.error("请选择 Dataset 并填写显示名称");
      return;
    }
    onInsertDatabase(parsed.data.dataset_id, parsed.data.display_text);
    close();
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>插入行内提及</DialogTitle>
          <DialogDescription>引用人员、日期或 Dataset。</DialogDescription>
        </DialogHeader>

        <div className="grid grid-cols-3 gap-1 rounded-md bg-muted p-1">
          <Button
            type="button"
            size="sm"
            variant={mode === "person" ? "default" : "ghost"}
            onClick={() => setMode("person")}
          >
            <UserRound className="size-4" />
            Person
          </Button>
          <Button
            type="button"
            size="sm"
            variant={mode === "date" ? "default" : "ghost"}
            onClick={() => setMode("date")}
          >
            <CalendarDays className="size-4" />
            Date
          </Button>
          <Button
            type="button"
            size="sm"
            variant={mode === "database" ? "default" : "ghost"}
            onClick={() => setMode("database")}
          >
            <Database className="size-4" />
            Database
          </Button>
        </div>

        {mode === "person" ? (
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="mention-actor-id">Actor ID</Label>
              <Input
                id="mention-actor-id"
                value={actorId}
                onChange={(event) => setActorId(event.target.value)}
                placeholder="00000000-0000-0000-0000-000000000000"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="mention-person-text">显示名称</Label>
              <Input
                id="mention-person-text"
                value={personText}
                onChange={(event) => setPersonText(event.target.value)}
              />
            </div>
          </div>
        ) : null}

        {mode === "date" ? (
          <div className="grid gap-4">
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-2">
                <Label htmlFor="mention-date">开始日期</Label>
                <Input
                  id="mention-date"
                  type="date"
                  value={date}
                  onChange={(event) => setDate(event.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="mention-end-date">结束日期</Label>
                <Input
                  id="mention-end-date"
                  type="date"
                  value={endDate}
                  min={date || undefined}
                  onChange={(event) => setEndDate(event.target.value)}
                />
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="mention-date-text">显示文本</Label>
              <Input
                id="mention-date-text"
                value={dateText}
                onChange={(event) => setDateText(event.target.value)}
                placeholder={endDate ? `${date} 至 ${endDate}` : date}
              />
            </div>
          </div>
        ) : null}

        {mode === "database" ? (
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="mention-dataset">Dataset</Label>
              <select
                id="mention-dataset"
                value={datasetId}
                onChange={(event) => {
                  setDatasetId(event.target.value);
                  const selected = datasets.find(
                    (dataset) => dataset.id === event.target.value,
                  );
                  setDatabaseText(selected?.name ?? "");
                }}
                className="border-input bg-background h-9 rounded-md border px-3 text-sm"
              >
                <option value="">
                  {datasetsLoading ? "加载中..." : "选择 Dataset"}
                </option>
                {datasets.map((dataset) => (
                  <option key={dataset.id} value={dataset.id}>
                    {dataset.name}
                  </option>
                ))}
              </select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="mention-database-text">显示名称</Label>
              <Input
                id="mention-database-text"
                value={databaseText}
                onChange={(event) => setDatabaseText(event.target.value)}
              />
            </div>
          </div>
        ) : null}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={close}>
            取消
          </Button>
          <Button type="button" onClick={submit}>
            插入
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
