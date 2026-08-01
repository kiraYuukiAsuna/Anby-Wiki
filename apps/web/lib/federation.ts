import type {
  EntityFederationLink,
  FederatedWiki,
} from "../../../contracts/generated/typescript";

export type FederationRelation = EntityFederationLink["relationType"];
export type FederationVerification =
  EntityFederationLink["verificationStatus"];
export type FederationLinkStatus = EntityFederationLink["status"];
export type FederationTrust = FederatedWiki["trustLevel"];
export type FederatedWikiStatus = FederatedWiki["status"];

export const FEDERATION_RELATIONS: ReadonlyArray<{
  value: FederationRelation;
  label: string;
  detail: string;
}> = [
  { value: "same_as", label: "同一身份", detail: "两个 ID 指向同一个对象" },
  { value: "broader", label: "更宽泛", detail: "远端概念范围更宽" },
  { value: "narrower", label: "更具体", detail: "远端概念范围更窄" },
  { value: "related", label: "相关", detail: "有明确关联但并非同一身份" },
];

export const FEDERATION_VERIFICATIONS: ReadonlyArray<{
  value: FederationVerification;
  label: string;
}> = [
  { value: "unverified", label: "待核验" },
  { value: "human_verified", label: "人工核验" },
  { value: "disputed", label: "有争议" },
];

export const FEDERATION_TRUST_LEVELS: ReadonlyArray<{
  value: FederationTrust;
  label: string;
  detail: string;
}> = [
  { value: "trusted", label: "可信", detail: "可作为高置信身份参照" },
  { value: "reference", label: "参考", detail: "作为普通外部参照" },
  { value: "untrusted", label: "未受信任", detail: "仅展示，不提升事实置信度" },
];

export const RELATION_LABEL: Record<FederationRelation, string> = {
  same_as: "同一身份",
  broader: "更宽泛",
  narrower: "更具体",
  related: "相关",
};

export const VERIFICATION_LABEL: Record<FederationVerification, string> = {
  unverified: "待核验",
  human_verified: "人工核验",
  disputed: "有争议",
};

export const TRUST_LABEL: Record<FederationTrust, string> = {
  trusted: "可信",
  reference: "参考",
  untrusted: "未受信任",
};
