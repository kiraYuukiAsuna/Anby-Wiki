import { Images } from "lucide-react";

import { AssetLibrary } from "@/components/assets/asset-library";

export default function AssetsPage() {
  return (
    <div className="mx-auto w-full max-w-7xl px-5 py-10 lg:px-8 lg:py-12">
      <header className="mb-9 border-b border-border/75 pb-8">
        <span className="flex size-11 items-center justify-center rounded-2xl bg-primary/9 text-primary">
          <Images className="size-5" aria-hidden />
        </span>
        <p className="mt-5 text-xs font-semibold tracking-[0.18em] text-primary uppercase">
          Media library
        </p>
        <h1 className="mt-2 text-4xl font-semibold tracking-[-0.045em]">
          媒体与附件
        </h1>
        <p className="mt-4 max-w-2xl text-sm leading-7 text-muted-foreground">
          集中管理 Wiki
          的图片、视频与附件。每次上传都形成不可变版本，让旧页面永远能还原当时引用的内容。
        </p>
      </header>
      <AssetLibrary />
    </div>
  );
}
