import { notFound } from "next/navigation";

import {
  ResponseError,
  type AssetRevision,
} from "../../../../../../contracts/generated/typescript";

import { AssetRevisionDetail } from "@/components/assets/asset-revision-detail";
import { assetsApi } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function AssetRevisionPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  let revision: AssetRevision;
  try {
    revision = await assetsApi().getAssetRevision({ revisionId: id });
  } catch (error) {
    if (error instanceof ResponseError && error.response.status === 404) {
      notFound();
    }
    throw error;
  }
  return <AssetRevisionDetail revision={revision} />;
}
