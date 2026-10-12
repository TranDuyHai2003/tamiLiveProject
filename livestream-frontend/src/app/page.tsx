import { RoomList } from "@/components/RoomList";

export default function HomePage() {
  return (
    <section>
      <h1 className="mb-6 text-2xl font-bold">Đang live</h1>
      <RoomList />
    </section>
  );
}
