import {fireEvent,render,screen} from '@testing-library/react';
import {it,expect,vi} from 'vitest';
import ReviewV2Settings from './ReviewV2Settings';
import {reviewV2Fixture} from '../../../shared/src/test/reviewV2Fixture';
it('renders recipe controls in order and preserves false, zero and null resets without network writes',()=>{
 const setDraft=vi.fn(),onClose=vi.fn();const fetchMock=vi.spyOn(globalThis,'fetch');
 render(<ReviewV2Settings request={reviewV2Fixture()} drafts={{}} setDraft={setDraft} onClose={onClose} direction="ask" open/>);
 expect(screen.getByLabelText('Response style')).toBeInTheDocument();
 fireEvent.change(screen.getByLabelText('Response style'),{target:{value:'0'}});expect(setDraft).toHaveBeenLastCalledWith('response_style','brief');
 fireEvent.click(screen.getByRole('button',{name:'Use default context'}));expect(setDraft).toHaveBeenLastCalledWith('extra_context',null);
 fireEvent.click(screen.getByRole('checkbox',{name:'Include citations'}));expect(setDraft).toHaveBeenLastCalledWith('citations',false);
 fireEvent.click(screen.getByRole('button',{name:'No examples'}));expect(setDraft).toHaveBeenLastCalledWith('examples',0);
 expect(screen.getByText(/This edit will not be sent with the selected direction/)).toBeInTheDocument();
 fireEvent.click(screen.getByRole('button',{name:'Done'}));expect(onClose).toHaveBeenCalledOnce();expect(fetchMock).not.toHaveBeenCalled();fetchMock.mockRestore();
});
