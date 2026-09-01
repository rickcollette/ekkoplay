import type {UploadProgress as Progress} from '../lib/api'

const size=(bytes:number)=>bytes>=1024**3?`${(bytes/1024**3).toFixed(1)} GB`:bytes>=1024**2?`${(bytes/1024**2).toFixed(1)} MB`:`${Math.ceil(bytes/1024)} KB`

export function UploadProgress({progress,count}:{progress:Progress;count:number}){return <section className="upload-progress" aria-live="polite"><div><strong>Uploading {count} {count===1?'file':'files'}</strong><span>{progress.percent}% · {size(progress.loaded)} of {size(progress.total)}</span></div><progress aria-label="Upload progress" max="100" value={progress.percent}>{progress.percent}%</progress></section>}
