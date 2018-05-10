vm kill all
vm flush
clear plumb
clear pipe

vm config filesystem /root/containerfs
vm launch container vm0
cc filter hostname=vm0
cc background /and net1 a0 x0
plumb net1 p0
vm launch container vm2
cc filter hostname=vm2
cc background /and net3 a1 x0
vm launch container vm4
cc filter hostname=vm4
cc background /and net5 a0 x1
vm launch container vm6
cc filter hostname=vm6
cc background /xor net7 net5 zero
vm launch container vm8
cc filter hostname=vm8
cc background /xor net9 net3 net7
vm launch container vm10
cc filter hostname=vm10
cc background /and net11 net3 net5
vm launch container vm12
cc filter hostname=vm12
cc background /or net13 net3 net5
vm launch container vm14
cc filter hostname=vm14
cc background /and net15 zero net13
vm launch container vm16
cc filter hostname=vm16
cc background /or net17 net11 net15
plumb net9 p1
vm launch container vm18
cc filter hostname=vm18
cc background /and net19 a2 x0
vm launch container vm20
cc filter hostname=vm20
cc background /and net21 a1 x1
vm launch container vm22
cc filter hostname=vm22
cc background /xor net23 net21 zero
vm launch container vm24
cc filter hostname=vm24
cc background /xor net25 net19 net23
vm launch container vm26
cc filter hostname=vm26
cc background /and net27 net19 net21
vm launch container vm28
cc filter hostname=vm28
cc background /or net29 net19 net21
vm launch container vm30
cc filter hostname=vm30
cc background /and net31 zero net29
vm launch container vm32
cc filter hostname=vm32
cc background /or net33 net27 net31
vm launch container vm34
cc filter hostname=vm34
cc background /and net35 a0 x2
vm launch container vm36
cc filter hostname=vm36
cc background /xor net37 net35 net17
vm launch container vm38
cc filter hostname=vm38
cc background /xor net39 net25 net37
vm launch container vm40
cc filter hostname=vm40
cc background /and net41 net25 net35
vm launch container vm42
cc filter hostname=vm42
cc background /or net43 net25 net35
vm launch container vm44
cc filter hostname=vm44
cc background /and net45 net17 net43
vm launch container vm46
cc filter hostname=vm46
cc background /or net47 net41 net45
plumb net39 p2
vm launch container vm48
cc filter hostname=vm48
cc background /and net49 a3 x0
vm launch container vm50
cc filter hostname=vm50
cc background /and net51 a2 x1
vm launch container vm52
cc filter hostname=vm52
cc background /xor net53 net51 zero
vm launch container vm54
cc filter hostname=vm54
cc background /xor net55 net49 net53
vm launch container vm56
cc filter hostname=vm56
cc background /and net57 net49 net51
vm launch container vm58
cc filter hostname=vm58
cc background /or net59 net49 net51
vm launch container vm60
cc filter hostname=vm60
cc background /and net61 zero net59
vm launch container vm62
cc filter hostname=vm62
cc background /or net63 net57 net61
vm launch container vm64
cc filter hostname=vm64
cc background /and net65 a1 x2
vm launch container vm66
cc filter hostname=vm66
cc background /xor net67 net65 net33
vm launch container vm68
cc filter hostname=vm68
cc background /xor net69 net55 net67
vm launch container vm70
cc filter hostname=vm70
cc background /and net71 net55 net65
vm launch container vm72
cc filter hostname=vm72
cc background /or net73 net55 net65
vm launch container vm74
cc filter hostname=vm74
cc background /and net75 net33 net73
vm launch container vm76
cc filter hostname=vm76
cc background /or net77 net71 net75
vm launch container vm78
cc filter hostname=vm78
cc background /and net79 a0 x3
vm launch container vm80
cc filter hostname=vm80
cc background /xor net81 net79 net47
vm launch container vm82
cc filter hostname=vm82
cc background /xor net83 net69 net81
vm launch container vm84
cc filter hostname=vm84
cc background /and net85 net69 net79
vm launch container vm86
cc filter hostname=vm86
cc background /or net87 net69 net79
vm launch container vm88
cc filter hostname=vm88
cc background /and net89 net47 net87
vm launch container vm90
cc filter hostname=vm90
cc background /or net91 net85 net89
plumb net83 p3
vm launch container vm92
cc filter hostname=vm92
cc background /and net93 a4 x0
vm launch container vm94
cc filter hostname=vm94
cc background /and net95 a3 x1
vm launch container vm96
cc filter hostname=vm96
cc background /xor net97 net95 zero
vm launch container vm98
cc filter hostname=vm98
cc background /xor net99 net93 net97
vm launch container vm100
cc filter hostname=vm100
cc background /and net101 net93 net95
vm launch container vm102
cc filter hostname=vm102
cc background /or net103 net93 net95
vm launch container vm104
cc filter hostname=vm104
cc background /and net105 zero net103
vm launch container vm106
cc filter hostname=vm106
cc background /or net107 net101 net105
vm launch container vm108
cc filter hostname=vm108
cc background /and net109 a2 x2
vm launch container vm110
cc filter hostname=vm110
cc background /xor net111 net109 net63
vm launch container vm112
cc filter hostname=vm112
cc background /xor net113 net99 net111
vm launch container vm114
cc filter hostname=vm114
cc background /and net115 net99 net109
vm launch container vm116
cc filter hostname=vm116
cc background /or net117 net99 net109
vm launch container vm118
cc filter hostname=vm118
cc background /and net119 net63 net117
vm launch container vm120
cc filter hostname=vm120
cc background /or net121 net115 net119
vm launch container vm122
cc filter hostname=vm122
cc background /and net123 a1 x3
vm launch container vm124
cc filter hostname=vm124
cc background /xor net125 net123 net77
vm launch container vm126
cc filter hostname=vm126
cc background /xor net127 net113 net125
vm launch container vm128
cc filter hostname=vm128
cc background /and net129 net113 net123
vm launch container vm130
cc filter hostname=vm130
cc background /or net131 net113 net123
vm launch container vm132
cc filter hostname=vm132
cc background /and net133 net77 net131
vm launch container vm134
cc filter hostname=vm134
cc background /or net135 net129 net133
vm launch container vm136
cc filter hostname=vm136
cc background /and net137 a0 x4
vm launch container vm138
cc filter hostname=vm138
cc background /xor net139 net137 net91
vm launch container vm140
cc filter hostname=vm140
cc background /xor net141 net127 net139
vm launch container vm142
cc filter hostname=vm142
cc background /and net143 net127 net137
vm launch container vm144
cc filter hostname=vm144
cc background /or net145 net127 net137
vm launch container vm146
cc filter hostname=vm146
cc background /and net147 net91 net145
vm launch container vm148
cc filter hostname=vm148
cc background /or net149 net143 net147
plumb net141 p4
vm launch container vm150
cc filter hostname=vm150
cc background /and net151 a4 x1
vm launch container vm152
cc filter hostname=vm152
cc background /and net153 a3 x2
vm launch container vm154
cc filter hostname=vm154
cc background /xor net155 net153 net107
vm launch container vm156
cc filter hostname=vm156
cc background /xor net157 net151 net155
vm launch container vm158
cc filter hostname=vm158
cc background /and net159 net151 net153
vm launch container vm160
cc filter hostname=vm160
cc background /or net161 net151 net153
vm launch container vm162
cc filter hostname=vm162
cc background /and net163 net107 net161
vm launch container vm164
cc filter hostname=vm164
cc background /or net165 net159 net163
vm launch container vm166
cc filter hostname=vm166
cc background /and net167 a2 x3
vm launch container vm168
cc filter hostname=vm168
cc background /xor net169 net167 net121
vm launch container vm170
cc filter hostname=vm170
cc background /xor net171 net157 net169
vm launch container vm172
cc filter hostname=vm172
cc background /and net173 net157 net167
vm launch container vm174
cc filter hostname=vm174
cc background /or net175 net157 net167
vm launch container vm176
cc filter hostname=vm176
cc background /and net177 net121 net175
vm launch container vm178
cc filter hostname=vm178
cc background /or net179 net173 net177
vm launch container vm180
cc filter hostname=vm180
cc background /and net181 a1 x4
vm launch container vm182
cc filter hostname=vm182
cc background /xor net183 net181 net135
vm launch container vm184
cc filter hostname=vm184
cc background /xor net185 net171 net183
vm launch container vm186
cc filter hostname=vm186
cc background /and net187 net171 net181
vm launch container vm188
cc filter hostname=vm188
cc background /or net189 net171 net181
vm launch container vm190
cc filter hostname=vm190
cc background /and net191 net135 net189
vm launch container vm192
cc filter hostname=vm192
cc background /or net193 net187 net191
vm launch container vm194
cc filter hostname=vm194
cc background /xor net195 zero net149
vm launch container vm196
cc filter hostname=vm196
cc background /xor net197 net185 net195
vm launch container vm198
cc filter hostname=vm198
cc background /and net199 net185 zero
vm launch container vm200
cc filter hostname=vm200
cc background /or net201 net185 zero
vm launch container vm202
cc filter hostname=vm202
cc background /and net203 net149 net201
vm launch container vm204
cc filter hostname=vm204
cc background /or net205 net199 net203
plumb net197 p5
vm launch container vm206
cc filter hostname=vm206
cc background /and net207 a4 x2
vm launch container vm208
cc filter hostname=vm208
cc background /and net209 a3 x3
vm launch container vm210
cc filter hostname=vm210
cc background /xor net211 net209 net165
vm launch container vm212
cc filter hostname=vm212
cc background /xor net213 net207 net211
vm launch container vm214
cc filter hostname=vm214
cc background /and net215 net207 net209
vm launch container vm216
cc filter hostname=vm216
cc background /or net217 net207 net209
vm launch container vm218
cc filter hostname=vm218
cc background /and net219 net165 net217
vm launch container vm220
cc filter hostname=vm220
cc background /or net221 net215 net219
vm launch container vm222
cc filter hostname=vm222
cc background /and net223 a2 x4
vm launch container vm224
cc filter hostname=vm224
cc background /xor net225 net223 net179
vm launch container vm226
cc filter hostname=vm226
cc background /xor net227 net213 net225
vm launch container vm228
cc filter hostname=vm228
cc background /and net229 net213 net223
vm launch container vm230
cc filter hostname=vm230
cc background /or net231 net213 net223
vm launch container vm232
cc filter hostname=vm232
cc background /and net233 net179 net231
vm launch container vm234
cc filter hostname=vm234
cc background /or net235 net229 net233
vm launch container vm236
cc filter hostname=vm236
cc background /xor net237 net205 net193
vm launch container vm238
cc filter hostname=vm238
cc background /xor net239 net227 net237
vm launch container vm240
cc filter hostname=vm240
cc background /and net241 net227 net205
vm launch container vm242
cc filter hostname=vm242
cc background /or net243 net227 net205
vm launch container vm244
cc filter hostname=vm244
cc background /and net245 net193 net243
vm launch container vm246
cc filter hostname=vm246
cc background /or net247 net241 net245
plumb net239 p6
vm launch container vm248
cc filter hostname=vm248
cc background /and net249 a4 x3
vm launch container vm250
cc filter hostname=vm250
cc background /and net251 a3 x4
vm launch container vm252
cc filter hostname=vm252
cc background /xor net253 net251 net221
vm launch container vm254
cc filter hostname=vm254
cc background /xor net255 net249 net253
vm launch container vm256
cc filter hostname=vm256
cc background /and net257 net249 net251
vm launch container vm258
cc filter hostname=vm258
cc background /or net259 net249 net251
vm launch container vm260
cc filter hostname=vm260
cc background /and net261 net221 net259
vm launch container vm262
cc filter hostname=vm262
cc background /or net263 net257 net261
vm launch container vm264
cc filter hostname=vm264
cc background /xor net265 net247 net235
vm launch container vm266
cc filter hostname=vm266
cc background /xor net267 net255 net265
vm launch container vm268
cc filter hostname=vm268
cc background /and net269 net255 net247
vm launch container vm270
cc filter hostname=vm270
cc background /or net271 net255 net247
vm launch container vm272
cc filter hostname=vm272
cc background /and net273 net235 net271
vm launch container vm274
cc filter hostname=vm274
cc background /or net275 net269 net273
plumb net267 p7
vm launch container vm276
cc filter hostname=vm276
cc background /and net277 a4 x4
vm launch container vm278
cc filter hostname=vm278
cc background /xor net279 net275 net263
vm launch container vm280
cc filter hostname=vm280
cc background /xor net281 net277 net279
vm launch container vm282
cc filter hostname=vm282
cc background /and net283 net277 net275
vm launch container vm284
cc filter hostname=vm284
cc background /or net285 net277 net275
vm launch container vm286
cc filter hostname=vm286
cc background /and net287 net263 net285
vm launch container vm288
cc filter hostname=vm288
cc background /or net289 net283 net287
plumb net281 p8
plumb net289 p9
vm start all
