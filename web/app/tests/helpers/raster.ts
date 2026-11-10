import { deflateSync } from 'node:zlib';
function crc(bytes: Buffer) { let c = 0xffffffff; for (const b of bytes) { c ^= b; for(let n=0;n<8;n++) c = (c>>>1) ^ ((c&1) ? 0xedb88320 : 0); } return (c ^ 0xffffffff) >>> 0; }
export function png(width=120,height=80,rgb=[35,110,190]) {
  const chunk=(tag:string,data:Buffer)=>{const type=Buffer.from(tag); const result=Buffer.alloc(data.length+12);result.writeUInt32BE(data.length);type.copy(result,4);data.copy(result,8);result.writeUInt32BE(crc(Buffer.concat([type,data])),result.length-4);return result;};
  const header=Buffer.alloc(13);header.writeUInt32BE(width);header.writeUInt32BE(height,4);header[8]=8;header[9]=2;
  const pixels=Buffer.alloc((width*3+1)*height);for(let y=0;y<height;y++)for(let x=0;x<width;x++)for(let c=0;c<3;c++)pixels[y*(width*3+1)+1+x*3+c]=rgb[c];
  return Buffer.concat([Buffer.from([137,80,78,71,13,10,26,10]),chunk('IHDR',header),chunk('IDAT',deflateSync(pixels)),chunk('IEND',Buffer.alloc(0))]);
}
export function paddedPNG(size:number) {
 const original=png(1,1);const length=size-original.length-12;if(length<8)throw new Error('Padding size too small');
 const type=Buffer.from('tEXt'),data=Buffer.alloc(length,32);data.write('Comment\0');const chunk=Buffer.alloc(length+12);chunk.writeUInt32BE(length);type.copy(chunk,4);data.copy(chunk,8);chunk.writeUInt32BE(crc(Buffer.concat([type,data])),length+8);
 return Buffer.concat([original.subarray(0,original.length-12),chunk,original.subarray(original.length-12)]);
}
