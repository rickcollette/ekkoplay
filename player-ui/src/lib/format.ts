export function duration(ms:number){const s=Math.max(0,Math.floor(ms/1000)),m=Math.floor(s/60);return `${m}:${String(s%60).padStart(2,'0')}`}
