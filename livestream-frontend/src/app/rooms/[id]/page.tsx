import { Suspense } from "react";

import { RoomScreen } from "@/components/room/RoomScreen";

// Với Cache Components, `params` của route động là dữ liệu chỉ có lúc request:
// phải đọc nó bên trong <Suspense>, nếu không build sẽ báo lỗi.
export default function RoomPage({ params }: PageProps<"/rooms/[id]">) {
  return (
    <Suspense fallback={<p className="text-neutral-500">Đang vào phòng…</p>}>
      <RoomFromParams params={params} />
    </Suspense>
  );
}

async function RoomFromParams({ params }: { params: PageProps<"/rooms/[id]">["params"] }) {
  const { id } = await params;
  return <RoomScreen roomId={id} />;
}
