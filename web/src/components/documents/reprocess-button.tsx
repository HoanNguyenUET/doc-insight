import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { useReprocessDocument } from "@/lib/queries";

// ReprocessButton xử lý lại tài liệu từ file PDF gốc đã lưu trên máy chủ.
export function ReprocessButton({ id, label = "Xử lý lại" }: { id: string; label?: string }) {
  const reprocess = useReprocessDocument();
  return (
    <Button
      disabled={reprocess.isPending}
      onClick={() =>
        reprocess.mutate(id, {
          onSuccess: () => toast("Đang xử lý lại tài liệu", { description: "Thường mất vài giây đến vài phút." }),
          onError: (err) => toast.error("Không xử lý lại được", { description: err.message }),
        })
      }
    >
      {reprocess.isPending ? "Đang gửi…" : label}
    </Button>
  );
}
