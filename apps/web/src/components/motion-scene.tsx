"use client";
import { useEffect, useRef, useState } from "react";
import { Pause, Play, Waves } from "lucide-react";
import { useSettings } from "@/lib/settings";

export function MotionScene({ compact = false }: { compact?: boolean }) {
  const video = useRef<HTMLVideoElement>(null);
  const [playing, setPlaying] = useState(false);
  const settings = useSettings();
  const motion = settings.data?.personalization.motion;
  useEffect(() => {
    if (!motion) return;
    const media = window.matchMedia("(prefers-reduced-motion: reduce)");
    const sync = () => {
      if (media.matches || motion === "reduce") video.current?.pause();
      else void video.current?.play().catch(() => {});
    };
    sync();
    media.addEventListener("change", sync);
    return () => media.removeEventListener("change", sync);
  }, [motion]);
  return (
    <section
      className={`motion-scene${compact ? " compact" : ""}`}
      aria-label="Анимация для спокойного фокуса"
    >
      <video
        ref={video}
        muted
        loop
        playsInline
        preload="metadata"
        poster="/art/orbit-poster.svg"
        onPlay={() => setPlaying(true)}
        onPause={() => setPlaying(false)}
        aria-hidden="true"
      >
        <source src="/art/orbit-loop.webm" type="video/webm" />
      </video>
      <div className="scene-caption">
        <span>
          <Waves size={16} aria-hidden="true" /> Немного пространства
        </span>
        <button
          type="button"
          className="scene-control"
          aria-label={playing ? "Остановить анимацию" : "Включить анимацию"}
          onClick={() => {
            if (playing) video.current?.pause();
            else void video.current?.play().catch(() => {});
          }}
        >
          {playing ? <Pause size={15} aria-hidden="true" /> : <Play size={15} aria-hidden="true" />}
        </button>
      </div>
    </section>
  );
}
