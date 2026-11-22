import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { message } from 'antd';
import { APIError, clearCrashConcern } from '@colony2/shared/api';
import type { RecipeJob } from '@colony2/shared/types';
import JobCrashConcern from './JobCrashConcern';
vi.mock('@colony2/shared/api',async original=>({...await original<typeof import('@colony2/shared/api')>(),clearCrashConcern:vi.fn()}));
const concern={job_id:'job',tenant_id:'tenant',status:'CRASH_CONCERN'} as RecipeJob;
const props=()=>({projectId:'tenant',jobId:'job',job:concern,status:'loaded' as const,refresh:vi.fn().mockResolvedValue({...concern,status:'WAITING'}),pause:vi.fn(),resume:vi.fn()});
const deferred=()=>{let resolve!:(v?:any)=>void;let reject!:(e:Error)=>void;const promise=new Promise<any>((a,b)=>{resolve=a;reject=b;});return {promise,resolve,reject};};
beforeEach(()=>{vi.mocked(clearCrashConcern).mockReset().mockResolvedValue(undefined);});
afterEach(()=>{cleanup();message.destroy();vi.restoreAllMocks();});
async function confirm(){fireEvent.click(await screen.findByRole('button',{name:'Clear crash concern',exact:true}));fireEvent.click(within(await screen.findByRole('dialog')).getByRole('button',{name:'Clear crash concern',exact:true}));}
it.each(['READY','ACTIVE','WAITING','COMPLETED','CANCELLED','PENDING_JOBS','AWAITING_FUTURE','FAILED_APP'])('does not derive a concern from %s or old outcomes',status=>{
 render(<JobCrashConcern {...props()} job={{...concern,status:status as RecipeJob['status'],completion_status:'failed_system'}} />);
 expect(screen.queryByRole('button',{name:'Clear crash concern'})).toBeNull();
});
it('withholds clear when metadata failed even if an old concern is retained',()=>{
 render(<JobCrashConcern {...props()} status="failed" />);expect(screen.queryByRole('button',{name:'Clear crash concern'})).toBeNull();
 expect(screen.getByText('Current crash concern status unavailable')).toBeVisible();
});
it('keeps the focused trigger through a background read while preventing stale clearing',async()=>{
 const p=props();const view=render(<JobCrashConcern {...p}/>);
 const trigger=screen.getByRole('button',{name:'Clear crash concern'});trigger.focus();
 view.rerender(<JobCrashConcern {...p} status="loading"/>);
 expect(screen.getByRole('button',{name:'Clear crash concern'})).toBe(trigger);
 expect(trigger).toHaveAttribute('aria-disabled','true');expect(trigger).toHaveFocus();fireEvent.click(trigger);
 expect(screen.queryByRole('dialog')).toBeNull();expect(clearCrashConcern).not.toHaveBeenCalled();
 view.rerender(<JobCrashConcern {...p}/>);
 expect(screen.getByRole('button',{name:'Clear crash concern'})).toBe(trigger);
 expect(trigger).toHaveAttribute('aria-disabled','false');expect(trigger).toHaveFocus();
 fireEvent.click(trigger);expect(await screen.findByRole('dialog')).toBeVisible();
});
it('invalidates reads before mutation, guards duplicate confirms, then reconciles independently',async()=>{
 const mutation=deferred(),read=deferred(),p=props();p.refresh.mockReturnValue(read.promise);vi.mocked(clearCrashConcern).mockReturnValue(mutation.promise);const toast=vi.spyOn(message,'success');
 render(<JobCrashConcern {...p}/>);await confirm();
 expect(p.pause).toHaveBeenCalledTimes(1);expect(p.refresh).not.toHaveBeenCalled();
 fireEvent.click(within(screen.getByRole('dialog')).getByRole('button',{name:'Clear crash concern'}));expect(clearCrashConcern).toHaveBeenCalledTimes(1);
 expect(within(screen.getByRole('dialog')).getByRole('status')).toHaveTextContent('Clearing');expect(within(screen.getByRole('dialog')).queryByRole('button',{name:'Close'})).toBeNull();
 await act(async()=>{mutation.resolve();});expect(toast).toHaveBeenCalledTimes(1);expect(p.refresh).toHaveBeenCalledTimes(1);expect(p.resume).not.toHaveBeenCalled();
 await act(async()=>{read.resolve();});expect(p.resume).toHaveBeenCalledTimes(1);
});
it.each([new APIError('state changed',409),new APIError('denied',403),new APIError('unsupported',501),new Error('lost response')])('refreshes after $message without retry or success',async error=>{
 const p=props(),toast=vi.spyOn(message,'success');vi.mocked(clearCrashConcern).mockRejectedValue(error);render(<JobCrashConcern {...p}/>);await confirm();
 await waitFor(()=>expect(p.resume).toHaveBeenCalledTimes(1));expect(p.refresh).toHaveBeenCalledTimes(1);expect(clearCrashConcern).toHaveBeenCalledTimes(1);expect(toast).not.toHaveBeenCalled();expect(screen.getByText('Could not confirm clearing')).toBeVisible();
});
it.each(['success','failure'] as const)('suppresses a late mutation %s after navigation or unmount',async outcome=>{
 const d=deferred(),p=props(),toast=vi.spyOn(message,'success');vi.mocked(clearCrashConcern).mockReturnValue(d.promise);const view=render(<JobCrashConcern key="old" {...p}/>);await confirm();
 view.rerender(<JobCrashConcern key="new" {...p} projectId="other" jobId="other-job" job={{...concern,job_id:'other-job',tenant_id:'other',status:'READY'}}/>);
 await act(async()=>{if(outcome==='success')d.resolve();else d.reject(new Error('late'));});expect(toast).not.toHaveBeenCalled();expect(p.refresh).not.toHaveBeenCalled();expect(screen.queryByRole('dialog')).toBeNull();expect(screen.queryByText('Could not confirm clearing')).toBeNull();
});
it('suppresses mutation callbacks after unmount',async()=>{
 const d=deferred(),p=props(),toast=vi.spyOn(message,'success');vi.mocked(clearCrashConcern).mockReturnValue(d.promise);const view=render(<JobCrashConcern {...p}/>);await confirm();view.unmount();await act(async()=>{d.resolve();});expect(toast).not.toHaveBeenCalled();expect(p.refresh).not.toHaveBeenCalled();
});
it('accepts renewed concern after exact reconciliation without inventing a ready status',async()=>{
 const p=props();const view=render(<JobCrashConcern {...p}/>);await confirm();await waitFor(()=>expect(p.resume).toHaveBeenCalled());view.rerender(<JobCrashConcern {...p} job={{...concern,status:'WAITING'}}/>);expect(within(view.container).queryByRole('button',{name:'Clear crash concern'})).toBeNull();view.rerender(<JobCrashConcern {...p}/>);expect(within(view.container).getByRole('button',{name:'Clear crash concern'})).toBeEnabled();
});
