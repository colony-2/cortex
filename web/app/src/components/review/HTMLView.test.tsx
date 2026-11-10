import {afterEach,beforeEach,it,expect,vi} from 'vitest';
import {render,screen,fireEvent,act,cleanup} from '@testing-library/react';
import HTMLView from './HTMLView';
import * as preparation from './htmlPreparation';
import type {DocumentData} from './document';
const source=(text:string):DocumentData=>({text,bytes:new TextEncoder().encode(text),format:'html',truncated:false});
const deferred=()=>{let resolve!:(v:preparation.PreparedHTML)=>void;const promise=new Promise<preparation.PreparedHTML>(r=>resolve=r);return {promise,resolve};};
const prepared=(content='<!doctype html><h1>Ready</h1>')=>({content,warnings:[],imageCount:0,imageBytes:0,outputBytes:content.length});
beforeEach(()=>{vi.spyOn(preparation,'nativeHTMLSupported').mockReturnValue(true);const OriginalURL=URL;vi.stubGlobal('URL',class extends OriginalURL{static createObjectURL=vi.fn(()=>'blob:prepared');static revokeObjectURL=vi.fn();});});
afterEach(()=>{cleanup();vi.mocked(preparation.nativeHTMLSupported).mockRestore();if(vi.isMockFunction(preparation.prepareHTML))vi.mocked(preparation.prepareHTML).mockRestore();vi.unstubAllGlobals();vi.useRealTimers();});
it('HP-016 aborts preparation on Source and rejects late completion',async()=>{
 const old=deferred();const prepare=vi.spyOn(preparation,'prepareHTML').mockReturnValue(old.promise);
 const {container}=render(<HTMLView name="page.html" data={source('<h1>Original</h1>')} />);
 const signal=prepare.mock.calls[0][2];fireEvent.click(screen.getByRole('radio',{name:'Source',exact:true}));expect(signal.aborted).toBe(true);
 await act(async()=>old.resolve(prepared()));expect(container.querySelector('iframe')).toBeNull();expect(URL.createObjectURL).not.toHaveBeenCalled();expect(screen.getByLabelText('Document source')).toHaveTextContent('Original');
});
it('HP-016 rejects replaced documents and aborts/revokes on unmount',async()=>{
 const old=deferred(),current=deferred();const prepare=vi.spyOn(preparation,'prepareHTML').mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
 const {container,rerender,unmount}=render(<HTMLView name="old.html" data={source('old')} />);const signal=prepare.mock.calls[0][2];
 rerender(<HTMLView name="new.html" data={source('new')} />);expect(signal.aborted).toBe(true);
 await act(async()=>old.resolve(prepared('obsolete')));expect(container.querySelector('iframe')).toBeNull();
 await act(async()=>current.resolve(prepared('current')));const frame=container.querySelector('iframe')!;expect(frame.title).toBe('Interactive HTML preview: new.html');expect(frame).toHaveAttribute('csp',preparation.INTERACTIVE_POLICY);expect(frame).toHaveAttribute('sandbox','allow-scripts');
 const activeSignal=prepare.mock.calls[1][2];unmount();expect(activeSignal.aborted).toBe(true);expect(frame.isConnected).toBe(false);expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:prepared');
});
it('HP-018 frame errors preserve Source and retry creates a fresh current frame',async()=>{
 vi.spyOn(preparation,'prepareHTML').mockResolvedValue(prepared());const original=source('<h1>Original</h1>');const {container}=render(<HTMLView name="page.html" data={original}/>);
 await act(async()=>{});const old=container.querySelector('iframe')!;fireEvent.error(old);expect(old.isConnected).toBe(false);expect(screen.getByLabelText('Document source')).toHaveTextContent(original.text);
 fireEvent.click(screen.getByRole('button',{name:'Retry rendering'}));await act(async()=>{});expect(container.querySelectorAll('iframe')).toHaveLength(1);expect(container.querySelector('iframe')).not.toBe(old);
});
it('HP-018 times out preparation, aborts work and prevents late revival',async()=>{
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout']});const old=deferred();const prepare=vi.spyOn(preparation,'prepareHTML').mockReturnValue(old.promise);const {container}=render(<HTMLView name="page.html" data={source('original')}/>);
 await act(async()=>{await vi.advanceTimersByTimeAsync(15_000)});expect(screen.getByText(/preparation timed out/)).toBeVisible();expect(prepare.mock.calls[0][2].aborted).toBe(true);
 await act(async()=>old.resolve(prepared()));expect(container.querySelector('iframe')).toBeNull();expect(URL.createObjectURL).not.toHaveBeenCalled();
});
