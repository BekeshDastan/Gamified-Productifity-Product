import { Planning } from "@/components/planning";
export default function PlanningPage() {
  return (
    <>
      <p className="eyebrow">ВРЕМЯ ДЛЯ ВАЖНОГО</p>
      <h1 className="page-title">План дня и недели</h1>
      <p className="page-intro">
        Выберите главное, оставьте место для отдыха и подтвердите посильный план.
      </p>
      <Planning />
    </>
  );
}
