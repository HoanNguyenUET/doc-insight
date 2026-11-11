import { useParams } from "react-router";

import { DocumentContent } from "@/components/content/document-content";

export function ContentPage() {
  const { id = "" } = useParams();
  return (
    <div className="flex-1 overflow-y-auto px-9 py-[30px]">
      <DocumentContent id={id} />
    </div>
  );
}
