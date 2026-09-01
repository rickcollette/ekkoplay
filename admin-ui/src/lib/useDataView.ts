import {useEffect,useMemo,useState} from 'react'
export type SortOption<T>={value:string;label:string;compare:(a:T,b:T)=>number}
export function useDataView<T>(items:T[],searchText:(item:T)=>string,sorts:SortOption<T>[],initialSort=sorts[0]?.value||''){
 const [query,setQuery]=useState(''),[sort,setSort]=useState(initialSort),[descending,setDescending]=useState(false),[page,setPage]=useState(1),[pageSize,setPageSize]=useState(25)
 useEffect(()=>setPage(1),[query,sort,descending,pageSize,items.length])
 const filtered=useMemo(()=>{const q=query.trim().toLowerCase(),option=sorts.find(x=>x.value===sort);const out=items.filter(item=>!q||searchText(item).toLowerCase().includes(q));if(option)out.sort((a,b)=>(descending?-1:1)*option.compare(a,b));return out},[items,query,sort,descending,searchText,sorts])
 const pages=Math.max(1,Math.ceil(filtered.length/pageSize)),safePage=Math.min(page,pages),visible=filtered.slice((safePage-1)*pageSize,safePage*pageSize)
 return{query,setQuery,sort,setSort,descending,setDescending,page:safePage,setPage,pageSize,setPageSize,total:filtered.length,pages,visible}
}
