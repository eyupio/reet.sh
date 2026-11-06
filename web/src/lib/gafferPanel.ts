export type GafferPanelSize='compact'|'comfortable'|'expanded'
export type GafferPanelSide='left'|'right'
export function readGafferLayout(value:string|null):{size:GafferPanelSize;side:GafferPanelSide}{
  try{const saved=JSON.parse(value||'{}');return {size:['compact','comfortable','expanded'].includes(saved?.size)?saved.size:'comfortable',side:saved?.side==='left'?'left':'right'}}catch{return {size:'comfortable',side:'right'}}
}

// Only presets persist. Dragging and the native resize grip are temporary;
// choosing a preset/full screen or reopening starts from the saved corner.
export function moveGafferPanel(node:HTMLDialogElement,initial:{reset:string;modal:boolean}){
  let options=initial,placed:{x:number;y:number}|null=null,drag:{id:number;dx:number;dy:number}|null=null
  function place(x:number,y:number){const box=node.getBoundingClientRect();placed={x:Math.max(0,Math.min(x,innerWidth-box.width)),y:Math.max(0,Math.min(y,innerHeight-box.height))};Object.assign(node.style,{left:`${placed.x}px`,top:`${placed.y}px`,right:'auto',bottom:'auto'})}
  function reset(){placed=null;drag=null;for(const key of ['left','top','right','bottom','width','height'])node.style.removeProperty(key)}
  function down(event:PointerEvent){if(options.modal||event.button!==0||!(event.target instanceof Element))return;const control=event.target.closest('button,a,input,select,textarea');if(control?!control.matches('[data-gaffer-move]'):!event.target.closest('[data-gaffer-drag]'))return;const box=node.getBoundingClientRect();drag={id:event.pointerId,dx:event.clientX-box.left,dy:event.clientY-box.top};node.setPointerCapture(event.pointerId);event.preventDefault();node.querySelector<HTMLButtonElement>('[data-gaffer-move]')?.focus()}
  function move(event:PointerEvent){if(drag&&event.pointerId===drag.id)place(event.clientX-drag.dx,event.clientY-drag.dy)}
  function end(){drag=null}
  function key(event:KeyboardEvent){if(options.modal||!(event.target instanceof Element)||!event.target.closest('[data-gaffer-move]'))return;const d:Record<string,number[]>={ArrowLeft:[-1,0],ArrowRight:[1,0],ArrowUp:[0,-1],ArrowDown:[0,1]},step=d[event.key];if(!step)return;event.preventDefault();const box=node.getBoundingClientRect(),by=event.shiftKey?40:10;place(box.left+step[0]*by,box.top+step[1]*by)}
  function clamp(){if(placed&&!options.modal)place(placed.x,placed.y)}
  node.addEventListener('pointerdown',down);node.addEventListener('pointermove',move);node.addEventListener('pointerup',end);node.addEventListener('pointercancel',end);node.addEventListener('lostpointercapture',end);node.addEventListener('keydown',key);window.addEventListener('resize',clamp)
  const size=new ResizeObserver(clamp);size.observe(node)
  return {update(next:typeof initial){if(next.reset!==options.reset||next.modal!==options.modal)reset();options=next},destroy(){size.disconnect();window.removeEventListener('resize',clamp);node.removeEventListener('pointerdown',down);node.removeEventListener('pointermove',move);node.removeEventListener('pointerup',end);node.removeEventListener('pointercancel',end);node.removeEventListener('lostpointercapture',end);node.removeEventListener('keydown',key)}}
}
