import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import Home from "./page";

describe("Home", () => {
  it("渲染项目名占位首页", () => {
    render(<Home />);
    expect(
      screen.getByRole("heading", { name: "Anby Wiki" })
    ).toBeInTheDocument();
  });
});
