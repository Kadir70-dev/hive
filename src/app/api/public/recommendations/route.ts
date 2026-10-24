export const runtime='nodejs';
export async function POST(){return Response.json({error:'Recommendations temporarily unavailable during infrastructure migration.'},{status:503});}
