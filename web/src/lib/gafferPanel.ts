export type GafferPanelSize='compact'|'comfortable'|'expanded'
export type GafferPanelSide='left'|'right'
export function readGafferLayout(value:string|null):{size:GafferPanelSize;side:GafferPanelSide}{
  try{const saved=JSON.parse(value||'{}');return {size:['compact','comfortable','expanded'].includes(saved?.size)?saved.size:'comfortable',side:saved?.side==='left'?'left':'right'}}catch{return {size:'comfortable',side:'right'}}
}

// Only presets persist. Dragging, the native resize grip and a snap back are
// temporary; choosing a preset/full screen or reopening starts from the saved
// corner. The whole header drags from any empty stretch, while its tools keep
// their clicks. Double-click, or a `gaffer-place` event from the panel's own
// "snap back" tool, returns it to that corner.
export function moveGafferPanel(node:HTMLDialogElement,initial:{reset:string;modal:boolean}){
  let options=initial,placed:{x:number;y:number}|null=null,drag:{id:number;dx:number;dy:number;x:number;y:number}|null=null
  // A moved panel keeps its corner after the pointer leaves, until a preset,
  // full screen, reopen or an explicit snap back resets it.
  function place(x:number,y:number){
    const box=node.getBoundingClientRect()
    placed={x:Math.max(0,Math.min(x,document.documentElement.clientWidth-box.width)),y:Math.max(0,Math.min(y,innerHeight-box.height))}
    Object.assign(node.style,{left:`${placed.x}px`,top:`${placed.y}px`,right:'auto',bottom:'auto'})
    node.dataset.gafferPlaced='true'
  }
  function reset(){placed=null;drag=null;delete node.dataset.gafferPlaced;delete node.dataset.gafferDragging;for(const key of ['left','top','right','bottom','width','height'])node.style.removeProperty(key)}
  function control(target:EventTarget|null){return target instanceof Element?target.closest('button,a,input,select,textarea,[role=button]'):null}
  function down(event:PointerEvent){if(options.modal||event.button!==0||!(event.target instanceof Element))return;const hit=control(event.target);if(hit?!hit.matches('[data-gaffer-move]'):!event.target.closest('[data-gaffer-drag]'))return;const box=node.getBoundingClientRect();drag={id:event.pointerId,dx:event.clientX-box.left,dy:event.clientY-box.top,x:event.clientX,y:event.clientY};node.setPointerCapture(event.pointerId);node.dataset.gafferDragging='true';event.preventDefault();node.querySelector<HTMLButtonElement>('[data-gaffer-move]')?.focus()}
  function move(event:PointerEvent){if(drag&&event.pointerId===drag.id)place(event.clientX-drag.dx,event.clientY-drag.dy)}
  // The release point is authoritative, so a coalesced final move cannot leave
  // the panel behind the pointer. A click on the header is not a move, and must
  // not pin the panel where it already sat.
  function up(event:PointerEvent){if(drag&&event.pointerId===drag.id&&(Math.abs(event.clientX-drag.x)>3||Math.abs(event.clientY-drag.y)>3))place(event.clientX-drag.dx,event.clientY-drag.dy);end()}
  function end(){drag=null;delete node.dataset.gafferDragging}
  function snap(event:Event){
    if(options.modal)return
    // A double-click only snaps from the header's own drag area; selecting words
    // in a message must never throw the panel back to its corner. The pointer is
    // captured, so the event target is the panel: compare coordinates instead.
    if(event instanceof MouseEvent){
      const area=node.querySelector<HTMLElement>('[data-gaffer-drag]')
      if(!area)return
      const box=area.getBoundingClientRect()
      if(event.clientX<box.left||event.clientX>box.right||event.clientY<box.top||event.clientY>box.bottom)return
    }
    reset()
  }
  function key(event:KeyboardEvent){if(options.modal||!(event.target instanceof Element)||!event.target.closest('[data-gaffer-move]'))return;const d:Record<string,number[]>={ArrowLeft:[-1,0],ArrowRight:[1,0],ArrowUp:[0,-1],ArrowDown:[0,1]},step=d[event.key];if(!step)return;event.preventDefault();const box=node.getBoundingClientRect(),by=event.shiftKey?40:10;place(box.left+step[0]*by,box.top+step[1]*by)}
  function clamp(){if(placed&&!options.modal)place(placed.x,placed.y)}
  node.addEventListener('pointerdown',down);node.addEventListener('pointermove',move);node.addEventListener('pointerup',up);node.addEventListener('pointercancel',end);node.addEventListener('lostpointercapture',end);node.addEventListener('keydown',key);node.addEventListener('dblclick',snap);node.addEventListener('gaffer-place',snap);window.addEventListener('resize',clamp)
  const size=new ResizeObserver(clamp);size.observe(node)
  return {update(next:typeof initial){if(next.reset!==options.reset||next.modal!==options.modal)reset();options=next},destroy(){size.disconnect();window.removeEventListener('resize',clamp);node.removeEventListener('pointerdown',down);node.removeEventListener('pointermove',move);node.removeEventListener('pointerup',up);node.removeEventListener('pointercancel',end);node.removeEventListener('lostpointercapture',end);node.removeEventListener('keydown',key);node.removeEventListener('dblclick',snap);node.removeEventListener('gaffer-place',snap)}}
}
