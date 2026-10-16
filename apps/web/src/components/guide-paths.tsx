"use client";
import { useState } from "react";
import { ArrowRight, CheckCheck, Compass, Moon, Timer, type LucideIcon } from "lucide-react";
import { SafeLink } from "@/lib/forms/unsaved-changes";
import { ChoiceCards } from "./ui/choice-cards";

const paths: {
  value: string;
  label: string;
  description: string;
  icon: LucideIcon;
  tone: string;
  title: string;
  detail: string;
  steps: { title: string; text: string; href: string; action: string }[];
}[] = [
  {
    value: "start",
    label: "Начать с малого",
    description: "Одно дело и понятный следующий шаг",
    icon: Compass,
    tone: "mint",
    title: "Не нужно настраивать всё сразу",
    detail: "Выберите один шаг. К остальным можно вернуться в любое время.",
    steps: [
      {
        title: "Запишите одно дело",
        text: "Добавьте задачу, которую реально выполнить сегодня.",
        href: "/app",
        action: "Открыть мой день",
      },
      {
        title: "Оставьте место в расписании",
        text: "Выберите доступное время и задачи для плана дня.",
        href: "/app/planning",
        action: "Собрать план",
      },
      {
        title: "Закрепите небольшую привычку",
        text: "Задайте посильную норму и отмечайте фактическое выполнение.",
        href: "/app/habits",
        action: "Выбрать привычку",
      },
    ],
  },
  {
    value: "focus",
    label: "Войти в фокус",
    description: "Меньше переключений, больше внимания",
    icon: Timer,
    tone: "blue",
    title: "Одно дело на ближайший отрезок",
    detail: "Продолжительность выбираете вы. Таймер можно поставить на паузу.",
    steps: [
      {
        title: "Выберите задачу",
        text: "Откройте список и определите конкретное следующее действие.",
        href: "/app/quests",
        action: "Открыть задачи",
      },
      {
        title: "Запустите часы",
        text: "Выберите режим фокуса и начните с комфортного интервала.",
        href: "/app/focus",
        action: "Перейти к таймеру",
      },
      {
        title: "Отметьте результат",
        text: "Подтвердите фактическую работу в таймере. Если дело готово, завершите задачу отдельно.",
        href: "/app/quests",
        action: "Вернуться к задачам",
      },
    ],
  },
  {
    value: "reflect",
    label: "Завершить день",
    description: "Замечать прогресс и спокойно отдыхать",
    icon: Moon,
    tone: "gold",
    title: "Освободите голову перед отдыхом",
    detail: "Незавершённое можно перенести. Отдых не отнимает накопленный опыт.",
    steps: [
      {
        title: "Подведите итог",
        text: "Отметьте настроение, важные события и то, что хочется запомнить.",
        href: "/app/journal?section=evening",
        action: "Открыть вечерний обзор",
      },
      {
        title: "Посмотрите на свой ритм",
        text: "В отчёте — реальные задачи, привычки и время фокуса за выбранный период.",
        href: "/app/analytics",
        action: "Посмотреть прогресс",
      },
      {
        title: "Подготовьте завтра",
        text: "Учитывайте встречи и оставьте свободное время между делами.",
        href: "/app/calendar",
        action: "Открыть календарь",
      },
    ],
  },
];

export function GuidePaths() {
  const [selected, setSelected] = useState("start");
  const path = paths.find((item) => item.value === selected)!;
  return (
    <section className="guide-paths" aria-label="Быстрый маршрут по приложению">
      <ChoiceCards
        label="Что хочется сделать сейчас?"
        value={selected}
        options={paths}
        onChange={setSelected}
      />
      <div className="guide-route" key={path.value}>
        <div className="support-card-heading">
          <span className="support-icon" data-tone={path.tone}>
            <path.icon aria-hidden="true" />
          </span>
          <div>
            <h2>{path.title}</h2>
            <p className="ui-hint">{path.detail}</p>
          </div>
        </div>
        <ol className="guide-steps">
          {path.steps.map((step, index) => (
            <li key={step.href + index}>
              <span className="guide-step-number" aria-hidden="true">
                {index + 1}
              </span>
              <div>
                <h3>{step.title}</h3>
                <p>{step.text}</p>
                <SafeLink href={step.href}>
                  {step.action}
                  <ArrowRight size={16} aria-hidden="true" />
                </SafeLink>
              </div>
            </li>
          ))}
        </ol>
      </div>
      <SafeLink href="/app/settings" className="guide-settings">
        <CheckCheck size={20} aria-hidden="true" />
        <span>
          <strong>Сделайте пространство своим</strong>
          <small>Оформление, часовой пояс и напоминания</small>
        </span>
        <ArrowRight size={18} aria-hidden="true" />
      </SafeLink>
    </section>
  );
}
