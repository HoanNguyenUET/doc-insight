// Biểu tượng nét mảnh lấy đúng path từ docs/design/mockup.html.
import type { SVGProps } from "react";

type IconProps = SVGProps<SVGSVGElement>;

function Icon({ d, ...props }: IconProps & { d: string }) {
  return (
    <svg fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeLinecap="round" strokeLinejoin="round" {...props}>
      <path d={d} />
    </svg>
  );
}

export const DocsIcon = (p: IconProps) => <Icon d="M4 6h16M4 12h16M4 18h10" {...p} />;
export const AskIcon = (p: IconProps) => <Icon d="M8 12h8M8 8h8M21 12a9 9 0 11-3.5-7.1L21 4v8z" {...p} />;
export const QuoteAskIcon = (p: IconProps) => <Icon d="M8 12h8M21 12a9 9 0 11-3.5-7.1L21 4v8z" {...p} />;
export const UploadIcon = (p: IconProps) => <Icon d="M12 16V4m0 0L8 8m4-4l4 4M4 17v2a2 2 0 002 2h12a2 2 0 002-2v-2" {...p} />;
export const DropIcon = (p: IconProps) => <Icon d="M12 16V4m0 0L8 8m4-4l4 4M4 15v3a3 3 0 003 3h10a3 3 0 003-3v-3" {...p} />;
export const IndexIcon = (p: IconProps) => <Icon d="M4 5h16M4 10h16M4 15h10M4 20h7" {...p} />;
export const PlusIcon = (p: IconProps) => <Icon d="M12 5v14M5 12h14" {...p} />;
export const TrashIcon = (p: IconProps) => <Icon d="M5 7h14M9 7V5h6v2M7 7l1 13h8l1-13" {...p} />;
export const CloseIcon = (p: IconProps) => <Icon d="M6 6l12 12M18 6L6 18" {...p} />;
export const SendIcon = (p: IconProps) => <Icon d="M4 12h15M13 6l6 6-6 6" {...p} />;

export function SearchIcon(p: IconProps) {
  return (
    <svg fill="none" stroke="currentColor" viewBox="0 0 24 24" {...p}>
      <circle cx="11" cy="11" r="7" />
      <path d="M20 20l-3.5-3.5" />
    </svg>
  );
}
