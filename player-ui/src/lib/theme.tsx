import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'

export type ThemeMode='auto'|'light'|'dark'|'night'
type ThemeContextValue={mode:ThemeMode;resolved:'light'|'dark'|'night';setMode:(m:ThemeMode)=>void}
const ThemeContext=createContext<ThemeContextValue|null>(null)

function resolve(mode:ThemeMode):'light'|'dark'|'night'{
  if(mode!=='auto') return mode
  const h=new Date().getHours()
  if(h>=22 || h<6) return 'night'
  if(h>=18) return 'dark'
  if(window.matchMedia?.('(prefers-color-scheme: dark)').matches && h<8) return 'dark'
  return 'light'
}
export function ThemeProvider({children}:{children:ReactNode}){
  const [mode,setModeState]=useState<ThemeMode>(()=>(localStorage.getItem('ekkoplayer-theme') as ThemeMode)||'auto')
  const [resolved,setResolved]=useState(()=>resolve(mode))
  useEffect(()=>{const run=()=>setResolved(resolve(mode));run();const id=window.setInterval(run,60000);return()=>clearInterval(id)},[mode])
  useEffect(()=>{document.documentElement.dataset.theme=resolved;document.documentElement.style.colorScheme=resolved==='light'?'light':'dark'},[resolved])
  const setMode=(m:ThemeMode)=>{localStorage.setItem('ekkoplayer-theme',m);setModeState(m)}
  const value=useMemo(()=>({mode,resolved,setMode}),[mode,resolved])
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}
export function useTheme(){const x=useContext(ThemeContext);if(!x)throw new Error('ThemeProvider missing');return x}
