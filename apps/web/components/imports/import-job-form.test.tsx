import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ImportJobForm } from "./import-job-form";

const mocks = vi.hoisted(() => ({
  createImportJob: vi.fn(),
  createImportUploadJob: vi.fn(),
  push: vi.fn(),
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@/lib/api", () => ({
  importsApi: () => ({
    createImportJob: mocks.createImportJob,
    createImportUploadJob: mocks.createImportUploadJob,
  }),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: mocks.push }) }));
vi.mock("sonner", () => ({ toast: mocks.toast }));

const JOB_ID = "0198a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a06";

describe("ImportJobForm", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.createImportJob.mockResolvedValue({ id: JOB_ID });
    mocks.createImportUploadJob.mockResolvedValue({ id: JOB_ID });
  });

  it("经生成客户端创建受控 URL 导入", async () => {
    render(<ImportJobForm />);
    fireEvent.change(screen.getByLabelText("来源 URL"), { target: { value: "https://example.com/release" } });
    fireEvent.change(screen.getByLabelText("来源标题（可选）"), { target: { value: "Release" } });
    fireEvent.click(screen.getByRole("button", { name: "创建导入任务" }));

    await waitFor(() => expect(mocks.createImportJob).toHaveBeenCalled());
    expect(mocks.createImportJob.mock.calls[0][0]).toMatchObject({
      createImportJobRequest: {
        jobType: "source_import",
        config: { source: { kind: "url", url: "https://example.com/release" }, title: "Release" },
      },
    });
    expect(mocks.push).toHaveBeenCalledWith(`/imports/${JOB_ID}`);
  });

  it("校验并经生成 multipart 客户端提交文件", async () => {
    const { container } = render(<ImportJobForm />);
    fireEvent.click(screen.getByRole("button", { name: "上传文件" }));
    const file = new File(["<!doctype html><html></html>"], "release.html", { type: "text/html" });
    const input = screen.getByLabelText("来源文件");
    Object.defineProperty(input, "files", { configurable: true, value: [file] });
    fireEvent.change(input);
    fireEvent.submit(container.querySelector("form")!);

    await waitFor(() => expect(mocks.createImportUploadJob).toHaveBeenCalled());
    expect(mocks.createImportUploadJob.mock.calls[0][0]).toMatchObject({ file });
    expect(mocks.createImportJob).not.toHaveBeenCalled();
  });
});
