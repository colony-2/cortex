import { Button, Input, Modal, Typography } from 'antd';
import { validateReviewScalar, type ReviewV2Request } from '@colony2/shared';
export default function ReviewV2Settings({request, drafts, setDraft, open, onClose, direction}: {
  request:ReviewV2Request; drafts:Record<string,unknown>;setDraft:(id:string,value:unknown)=>void;open:boolean;onClose:()=>void;direction?:string;
}) {
  return <Modal title="Review settings" open={open} onCancel={onClose} styles={{body:{maxHeight:'65dvh',overflowY:'auto'}}} footer={<Button onClick={onClose}>Done</Button>}>
    <p>Edits are sent when you confirm a direction.</p>
    {(request.settings??[]).map(s=>{
      const value=Object.prototype.hasOwnProperty.call(drafts,s.id)?drafts[s.id]:s.value;
      const error=validateReviewScalar(value,s.schema);
      const choices=s.choices??[];
      return <section key={s.id} className="review-v2-setting">
        <label htmlFor={`review-setting-${s.id}`}><strong>{s.label}</strong></label>
        {s.description && <p>{s.description}</p>}
        {s.control==='text' && <Input id={`review-setting-${s.id}`} value={typeof value==='string'?value:''} placeholder={value===null?'Default':undefined} onChange={e=>setDraft(s.id,e.target.value)}/>}
        {s.control==='number' && <Input id={`review-setting-${s.id}`} type="number" value={typeof value==='number'||typeof value==='string'?value:''} onChange={e=>setDraft(s.id,e.target.value===''?'':Number(e.target.value))}/>}
        {s.control==='boolean' && <input type="checkbox" aria-label={s.label} id={`review-setting-${s.id}`} ref={el=>{if(el)el.indeterminate=value===null;}} style={{display:'block',marginTop:8,width:18,height:18}} checked={value===true} onChange={e=>setDraft(s.id,e.target.checked)}/>}
        {s.control==='choice' && <select id={`review-setting-${s.id}`} style={{width:'100%',minHeight:36,border:'1px solid #d9d9d9',borderRadius:6,padding:6,background:'white'}} value={choices.findIndex(c=>c.value===value)>=0?String(choices.findIndex(c=>c.value===value)):''} onChange={e=>setDraft(s.id,choices[Number(e.target.value)].value)}><option value="" disabled>{value===null?s.reset_label:'Choose a value'}</option>{choices.map((c,i)=><option key={i} value={String(i)}>{c.label}</option>)}</select>}
        {error && <p role="alert">{error}</p>}
        <p><Typography.Text type="secondary">{s.effect}</Typography.Text></p>
        <p>Applies to: {s.applies_to.map(id=>request.directions.find(d=>d.id===id)?.label??id).join(', ')}.{direction&&!s.applies_to.includes(direction)&&' This edit will not be sent with the selected direction.'}</p>
        {Object.prototype.hasOwnProperty.call(s,'default')&&<Button onClick={()=>setDraft(s.id,s.default)}>{s.reset_label}</Button>}
        <p>Current value: <code>{JSON.stringify(s.value)}</code></p>
      </section>;
    })}
  </Modal>;
}
