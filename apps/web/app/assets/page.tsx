import { Images } from "lucide-react";

import { AssetLibrary } from "@/components/assets/asset-library";

export default function AssetsPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-8 flex items-center gap-3 border-b border-border/75 pb-6">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-primary/9 text-primary">
          <Images className="size-5" aria-hidden />
        </span>
        <h1 className="text-3xl font-semibold">媒体与附件</h1>
      </header>
      <AssetLibrary />
    </div>
  );
}
