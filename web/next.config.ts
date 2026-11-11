import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Thư mục home có package-lock.json riêng; chỉ rõ root để Turbopack không
  // lấy nhầm thư mục cha làm workspace.
  turbopack: { root: __dirname },
};

export default nextConfig;
