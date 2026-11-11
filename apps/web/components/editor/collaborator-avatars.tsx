"use client";

import { Popover, Tooltip } from "radix-ui";
import type { CollaborationPresence } from "@/lib/collaboration/client";

const colors = ["bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-200", "bg-violet-100 text-violet-800 dark:bg-violet-950 dark:text-violet-200", "bg-teal-100 text-teal-800 dark:bg-teal-950 dark:text-teal-200", "bg-orange-100 text-orange-800 dark:bg-orange-950 dark:text-orange-200"];
const avatarClass = "relative flex size-7 shrink-0 items-center justify-center rounded-full border-2 border-background text-xs font-semibold outline-none hover:z-10 focus-visible:z-10 focus-visible:ring-2 focus-visible:ring-ring";
const panelClass = "z-50 max-w-[calc(100vw-2rem)] rounded-lg border bg-popover p-3 text-popover-foreground shadow-md";

export function presenceLabel(presence: CollaborationPresence): string {
  return presence.actorDisplayName || (presence.actorUsername ? `@${presence.actorUsername}` : `成员 ${presence.actorId.slice(0, 8)}`);
}

function Initial({ person }: { person: CollaborationPresence }) {
  const color = colors[Array.from(person.actorId).reduce((sum, char) => sum + char.charCodeAt(0), 0) % colors.length];
  return <span aria-hidden className={`flex size-full items-center justify-center rounded-full ${color}`}>{Array.from(presenceLabel(person).replace(/^@/, ""))[0]?.toUpperCase()}</span>;
}

function Identity({ person }: { person: CollaborationPresence }) {
  return <div className="min-w-0 max-w-72 break-words text-left">
    <p className="text-sm font-medium">{presenceLabel(person)}</p>
    {person.actorUsername && <p className="mt-0.5 text-xs text-muted-foreground">@{person.actorUsername}</p>}
    {person.actorEmail && <p className="mt-1 break-all text-xs text-muted-foreground">{person.actorEmail}</p>}
  </div>;
}

export function CollaboratorAvatars({ peers, actorId }: { peers: CollaborationPresence[]; actorId: string }) {
  const people = [...new Map(peers.filter(peer => peer.actorId !== actorId).map(peer => [peer.actorId, peer])).values()];
  if (!people.length) return null;
  const remaining = people.slice(4);
  return <div className="ml-1 flex -space-x-1.5" role="group" aria-label={`${people.length} 位协作者在线`} data-collaboration-presence>
    <Tooltip.Provider delayDuration={200}>
      {people.slice(0, 4).map(person => <Popover.Root key={person.actorId}>
        <Tooltip.Root>
          <Tooltip.Trigger asChild><Popover.Trigger asChild>
            <button type="button" className={avatarClass} aria-label={presenceLabel(person)} data-collaborator-id={person.actorId}><Initial person={person} /></button>
          </Popover.Trigger></Tooltip.Trigger>
          <Tooltip.Portal><Tooltip.Content side="bottom" sideOffset={6} collisionPadding={16} className={panelClass}><Identity person={person} /></Tooltip.Content></Tooltip.Portal>
        </Tooltip.Root>
        <Popover.Portal><Popover.Content side="bottom" align="start" sideOffset={6} collisionPadding={16} className={panelClass} aria-label={`${presenceLabel(person)}的资料`}><Identity person={person} /></Popover.Content></Popover.Portal>
      </Popover.Root>)}
    </Tooltip.Provider>
    {remaining.length > 0 && <Popover.Root>
      <Popover.Trigger asChild><button type="button" className={`${avatarClass} bg-muted text-muted-foreground`} aria-label={`查看其余 ${remaining.length} 位协作者`}>+{remaining.length}</button></Popover.Trigger>
      <Popover.Portal><Popover.Content side="bottom" align="end" sideOffset={6} collisionPadding={16} className={`${panelClass} max-h-72 overflow-y-auto`} aria-label="更多在线协作者">
        <ul className="space-y-3">{remaining.map(person => <li key={person.actorId} className="flex items-center gap-2"><span className="size-8 shrink-0"><Initial person={person} /></span><Identity person={person} /></li>)}</ul>
      </Popover.Content></Popover.Portal>
    </Popover.Root>}
  </div>;
}
